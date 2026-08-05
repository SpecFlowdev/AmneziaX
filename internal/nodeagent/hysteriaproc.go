package nodeagent

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	"log/slog"
)

// Hysteria supervises the hysteria2 process, beside the xray one rather than
// instead of it: a node can serve both, and the two share nothing but the fact
// that something has to keep them alive.
//
// It is deliberately a separate type from Xray rather than a refactor of it.
// The xray path is what every existing node runs, and generalising a working
// supervisor to fit a second engine is a change with no upside for the nodes
// already out there.
type Hysteria struct {
	binary     string
	workDir    string
	configName string
	args       []string
	log        *slog.Logger

	mu      sync.Mutex
	current *proc
	hash    string
}

func NewHysteria(binary, workDir string, log *slog.Logger) *Hysteria {
	return &Hysteria{binary: binary, workDir: workDir, log: log,
		configName: "hysteria.yaml", args: []string{"server", "-c"}}
}

// NewSingBox supervises sing-box with the same machinery. The two differ only
// in the binary, the file name and the arguments — everything about keeping a
// process alive and swapping its config is identical, and duplicating it would
// mean fixing the next bug twice.
func NewSingBox(binary, workDir string, log *slog.Logger) *Hysteria {
	return &Hysteria{binary: binary, workDir: workDir, log: log,
		configName: "singbox.json", args: []string{"run", "-c"}}
}

func (h *Hysteria) configPath() string { return filepath.Join(h.workDir, h.configName) }

// Available reports whether the binary is actually installed. A node that was
// enrolled before hysteria2 existed has no such binary, and the honest answer
// to a hysteria config arriving there is an error the panel can show — not a
// silent no-op that looks like success.
func (h *Hysteria) Available() bool {
	if h.binary == "" {
		return false
	}
	info, err := os.Stat(h.binary)
	return err == nil && !info.IsDir()
}

func (h *Hysteria) Running() bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.current == nil {
		return false
	}
	select {
	case <-h.current.exited:
		return false
	default:
		return true
	}
}

func (h *Hysteria) ConfigHash() string {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.hash
}

// Apply writes the config and restarts, unless the config is byte-identical to
// what is already running — restarting on an unchanged document would drop
// every live connection for nothing.
func (h *Hysteria) Apply(ctx context.Context, config []byte, hash string) error {
	if !h.Available() {
		return errors.New(h.name() + " is not installed on this node; re-run the installer to add it")
	}
	if hash == "" {
		sum := sha256.Sum256(config)
		hash = hex.EncodeToString(sum[:])
	}

	h.mu.Lock()
	unchanged := h.hash == hash && h.current != nil
	h.mu.Unlock()
	if unchanged && h.Running() {
		return nil
	}

	if err := os.MkdirAll(h.workDir, 0o750); err != nil {
		return err
	}
	// Written next to the target and renamed, so a crash midway cannot leave a
	// half-written config that the next start would refuse.
	tmp := h.configPath() + ".new"
	if err := os.WriteFile(tmp, config, 0o600); err != nil {
		return err
	}
	if err := os.Rename(tmp, h.configPath()); err != nil {
		return err
	}

	if err := h.restart(ctx); err != nil {
		return err
	}
	h.mu.Lock()
	h.hash = hash
	h.mu.Unlock()
	return nil
}

func (h *Hysteria) restart(ctx context.Context) error {
	h.Stop()

	cmd := exec.CommandContext(ctx, h.binary, append(append([]string{}, h.args...), h.configPath())...)
	cmd.Dir = h.workDir
	if err := cmd.Start(); err != nil {
		return err
	}

	p := &proc{cmd: cmd, startedAt: time.Now(), exited: make(chan struct{})}
	go func() {
		_ = cmd.Wait()
		close(p.exited)
	}()

	h.mu.Lock()
	h.current = p
	h.mu.Unlock()

	// hysteria has no offline config check, so the only way to know the
	// document was accepted is to watch whether the process survives its own
	// start-up. Exiting inside a second means it refused the config.
	select {
	case <-p.exited:
		return errors.New(h.name() + " exited immediately — the configuration was refused")
	case <-time.After(time.Second):
		return nil
	}
}

func (h *Hysteria) Stop() {
	h.mu.Lock()
	p := h.current
	h.current = nil
	h.mu.Unlock()
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

// name is what the engine is called in a message an operator reads.
func (h *Hysteria) name() string {
	if h.configName == "singbox.json" {
		return "sing-box"
	}
	return "hysteria2"
}
