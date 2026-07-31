// Package nodeagent runs xray-core on a node and keeps it aligned with the
// configuration the panel pushes.
package nodeagent

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// ringSize is how many recent xray log lines the agent keeps for the panel's
// log viewer. Older lines are dropped rather than written to disk.
const ringSize = 500

// Xray supervises the xray-core process.
type Xray struct {
	binary  string
	workDir string
	apiAddr string
	log     *slog.Logger

	mu      sync.Mutex
	current *proc
	hash    string

	ringMu sync.Mutex
	ring   []string
	ringAt int
}

// proc is one supervised xray process. exited closes when Wait returns, which
// is the single place the process is reaped.
type proc struct {
	cmd       *exec.Cmd
	startedAt time.Time
	exited    chan struct{}
}

func NewXray(binary, workDir, apiAddr string, log *slog.Logger) *Xray {
	return &Xray{
		binary:  binary,
		workDir: workDir,
		apiAddr: apiAddr,
		log:     log,
		ring:    make([]string, ringSize),
	}
}

func (x *Xray) configPath() string { return filepath.Join(x.workDir, "config.json") }

// Version reports the xray-core version, or "unknown" if the binary is missing.
func (x *Xray) Version() string {
	out, err := exec.Command(x.binary, "version").Output()
	if err != nil {
		return "unknown"
	}
	line := strings.SplitN(strings.TrimSpace(string(out)), "\n", 2)[0]
	fields := strings.Fields(line)
	if len(fields) >= 2 {
		return fields[1]
	}
	return strings.TrimSpace(line)
}

// Running reports whether a supervised process is currently alive.
func (x *Xray) Running() bool {
	x.mu.Lock()
	p := x.current
	x.mu.Unlock()
	if p == nil {
		return false
	}
	select {
	case <-p.exited:
		return false
	default:
		return true
	}
}

func (x *Xray) StartedAt() time.Time {
	x.mu.Lock()
	defer x.mu.Unlock()
	if x.current == nil {
		return time.Time{}
	}
	return x.current.startedAt
}

func (x *Xray) ConfigHash() string {
	x.mu.Lock()
	defer x.mu.Unlock()
	return x.hash
}

// LoadPersistedHash restores the hash of the config left on disk by a previous
// run, so a restarted agent can tell the panel it is already up to date.
func (x *Xray) LoadPersistedHash() {
	raw, err := os.ReadFile(filepath.Join(x.workDir, "config.hash"))
	if err != nil {
		return
	}
	x.mu.Lock()
	x.hash = strings.TrimSpace(string(raw))
	x.mu.Unlock()
}

// Apply writes a new configuration and restarts xray. The previous config is
// kept so a rejected document can be rolled back without contacting the panel.
func (x *Xray) Apply(ctx context.Context, config []byte, hash string) error {
	if !json.Valid(config) {
		return fmt.Errorf("panel sent an invalid JSON configuration")
	}
	if err := os.MkdirAll(x.workDir, 0o750); err != nil {
		return fmt.Errorf("create work dir: %w", err)
	}

	previous, prevErr := os.ReadFile(x.configPath())
	hasPrevious := prevErr == nil

	if err := os.WriteFile(x.configPath(), config, 0o600); err != nil {
		return fmt.Errorf("write config: %w", err)
	}

	if err := x.test(); err != nil {
		if hasPrevious {
			_ = os.WriteFile(x.configPath(), previous, 0o600)
		}
		return fmt.Errorf("configuration rejected by xray: %w", err)
	}

	if err := x.restart(ctx); err != nil {
		if hasPrevious {
			_ = os.WriteFile(x.configPath(), previous, 0o600)
			if rollbackErr := x.restart(ctx); rollbackErr != nil {
				x.log.Error("rollback to the previous configuration failed", "error", rollbackErr)
			}
		}
		return err
	}

	x.mu.Lock()
	x.hash = hash
	x.mu.Unlock()
	_ = os.WriteFile(filepath.Join(x.workDir, "config.hash"), []byte(hash), 0o600)
	return nil
}

// test runs xray's own config check so a bad document never takes the node down.
func (x *Xray) test() error {
	cmd := exec.Command(x.binary, "run", "-test", "-config", x.configPath())
	out, err := cmd.CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(out))
		if len(msg) > 500 {
			msg = msg[:500]
		}
		return fmt.Errorf("%s", msg)
	}
	return nil
}

// Start launches xray with whatever config is already on disk.
func (x *Xray) Start(ctx context.Context) error {
	if _, err := os.Stat(x.configPath()); err != nil {
		return fmt.Errorf("no configuration on disk yet")
	}
	return x.restart(ctx)
}

func (x *Xray) restart(ctx context.Context) error {
	x.Stop()

	cmd := exec.Command(x.binary, "run", "-config", x.configPath())
	cmd.Dir = x.workDir
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	cmd.Stderr = cmd.Stdout

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start xray: %w", err)
	}

	p := &proc{cmd: cmd, startedAt: time.Now(), exited: make(chan struct{})}
	x.mu.Lock()
	x.current = p
	x.mu.Unlock()

	go x.drainLogs(stdout)
	go func() {
		err := cmd.Wait()
		close(p.exited)
		if err != nil && ctx.Err() == nil {
			x.log.Warn("xray exited", "error", err)
			x.appendLog("xray exited: " + err.Error())
		}
	}()

	// Give xray a moment to fail fast on a port conflict or a bad certificate.
	time.Sleep(700 * time.Millisecond)
	if !x.Running() {
		return fmt.Errorf("xray exited immediately after start: %s", strings.Join(x.Logs(10), " | "))
	}
	return nil
}

// Stop terminates the process, escalating to SIGKILL if it ignores the first
// signal.
func (x *Xray) Stop() {
	x.mu.Lock()
	p := x.current
	x.current = nil
	x.mu.Unlock()

	if p == nil || p.cmd.Process == nil {
		return
	}
	_ = p.cmd.Process.Signal(os.Interrupt)
	select {
	case <-p.exited:
	case <-time.After(5 * time.Second):
		_ = p.cmd.Process.Kill()
		<-p.exited
	}
}

func (x *Xray) drainLogs(r interface{ Read([]byte) (int, error) }) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 1<<20)
	for scanner.Scan() {
		x.appendLog(scanner.Text())
	}
}

func (x *Xray) appendLog(line string) {
	x.ringMu.Lock()
	x.ring[x.ringAt%ringSize] = line
	x.ringAt++
	x.ringMu.Unlock()
}

// Logs returns the most recent n log lines, oldest first.
func (x *Xray) Logs(n int) []string {
	x.ringMu.Lock()
	defer x.ringMu.Unlock()

	total := x.ringAt
	if total > ringSize {
		total = ringSize
	}
	if n > total {
		n = total
	}
	out := make([]string, 0, n)
	for i := total - n; i < total; i++ {
		idx := (x.ringAt - total + i) % ringSize
		if idx < 0 {
			idx += ringSize
		}
		if line := x.ring[idx]; line != "" {
			out = append(out, line)
		}
	}
	return out
}
