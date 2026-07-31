package nodeagent

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"sync"
	"time"

	nodev1 "github.com/SpecFlowdev/AmneziaX/gen/go/node/v1"
	"github.com/SpecFlowdev/AmneziaX/internal/config"
	"github.com/SpecFlowdev/AmneziaX/internal/version"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/keepalive"
)

type Agent struct {
	cfg  *config.Agent
	xray *Xray
	log  *slog.Logger

	mu                sync.Mutex
	heartbeatInterval time.Duration
	usageInterval     time.Duration
	lastError         string
}

func New(cfg *config.Agent, log *slog.Logger) *Agent {
	return &Agent{
		cfg:               cfg,
		xray:              NewXray(cfg.XrayBinary, cfg.XrayWorkDir, cfg.XrayAPIAddr, log),
		log:               log,
		heartbeatInterval: 10 * time.Second,
		usageInterval:     30 * time.Second,
	}
}

// Run keeps a control stream to the panel open, reconnecting with backoff for
// as long as the process lives.
func (a *Agent) Run(ctx context.Context) error {
	a.xray.LoadPersistedHash()

	// A node that already has a config should serve traffic while it waits for
	// the panel to come back, rather than staying dark until reconnection.
	if err := a.xray.Start(ctx); err != nil {
		a.log.Info("xray not started yet", "reason", err)
	}
	defer a.xray.Stop()

	backoff := time.Second
	for {
		if ctx.Err() != nil {
			return nil
		}
		err := a.session(ctx)
		if ctx.Err() != nil {
			return nil
		}
		if err != nil {
			a.log.Warn("connection to the panel lost", "error", err, "retryIn", backoff)
		}
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(backoff):
		}
		backoff *= 2
		if backoff > 30*time.Second {
			backoff = 30 * time.Second
		}
	}
}

func (a *Agent) dial() (*grpc.ClientConn, error) {
	var creds credentials.TransportCredentials
	if a.cfg.Insecure {
		creds = insecure.NewCredentials()
	} else {
		creds = credentials.NewTLS(&tls.Config{ServerName: a.cfg.ServerName, MinVersion: tls.VersionTLS12})
	}
	return grpc.NewClient(a.cfg.PanelAddr,
		grpc.WithTransportCredentials(creds),
		grpc.WithDefaultCallOptions(grpc.MaxCallRecvMsgSize(16<<20)),
		grpc.WithKeepaliveParams(keepalive.ClientParameters{
			Time:                20 * time.Second,
			Timeout:             15 * time.Second,
			PermitWithoutStream: true,
		}),
	)
}

// session runs one connection: handshake, then concurrent send and receive
// loops until either side breaks.
func (a *Agent) session(parent context.Context) error {
	conn, err := a.dial()
	if err != nil {
		return fmt.Errorf("dial panel: %w", err)
	}
	defer conn.Close()

	ctx, cancel := context.WithCancel(parent)
	defer cancel()

	stream, err := nodev1.NewNodeControlClient(conn).Connect(ctx)
	if err != nil {
		return fmt.Errorf("open stream: %w", err)
	}

	// The gRPC client connects lazily, so an unreachable panel only shows up
	// once we try to talk. This watchdog tears the attempt down instead of
	// letting a black-holed address hang the agent forever.
	handshakeDone := make(chan struct{})
	go func() {
		select {
		case <-handshakeDone:
		case <-ctx.Done():
		case <-time.After(30 * time.Second):
			cancel()
		}
	}()

	snapshot := Snapshot()
	hello := &nodev1.Hello{
		NodeUuid:     a.cfg.NodeUUID,
		Token:        a.cfg.Token,
		AgentVersion: version.Version,
		XrayVersion:  a.xray.Version(),
		ConfigHash:   a.xray.ConfigHash(),
		System: &nodev1.SystemInfo{
			Hostname:      snapshot.Hostname,
			Os:            snapshot.OS,
			Arch:          snapshot.Arch,
			Kernel:        snapshot.Kernel,
			CpuCount:      uint32(snapshot.CPUCount),
			CpuModel:      snapshot.CPUModel,
			TotalRamBytes: snapshot.TotalRAMBytes,
		},
	}
	if err := stream.Send(&nodev1.AgentMessage{Payload: &nodev1.AgentMessage_Hello{Hello: hello}}); err != nil {
		return fmt.Errorf("send hello: %w", err)
	}

	first, err := stream.Recv()
	if err != nil {
		return fmt.Errorf("handshake: %w", err)
	}
	ack := first.GetHelloAck()
	if ack == nil {
		return errors.New("the panel did not acknowledge the handshake")
	}
	if !ack.GetAccepted() {
		// A rejected token will not fix itself; wait longer before retrying so
		// the agent does not hammer the panel.
		a.log.Error("the panel rejected this node", "reason", ack.GetError())
		select {
		case <-ctx.Done():
		case <-time.After(60 * time.Second):
		}
		return fmt.Errorf("rejected: %s", ack.GetError())
	}

	close(handshakeDone)

	a.mu.Lock()
	if s := ack.GetHeartbeatIntervalSeconds(); s > 0 {
		a.heartbeatInterval = time.Duration(s) * time.Second
	}
	if s := ack.GetUsageIntervalSeconds(); s > 0 {
		a.usageInterval = time.Duration(s) * time.Second
	}
	heartbeat, usage := a.heartbeatInterval, a.usageInterval
	a.mu.Unlock()

	a.log.Info("connected to the panel", "node", ack.GetNodeName(),
		"heartbeat", heartbeat, "usageReport", usage)

	// One goroutine owns the send side of the stream; everything that needs to
	// talk to the panel funnels through this channel.
	outbox := make(chan *nodev1.AgentMessage, 64)
	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()
		a.sendLoop(ctx, stream, outbox)
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		a.telemetryLoop(ctx, outbox, heartbeat, usage)
	}()

	err = a.receiveLoop(ctx, stream, outbox)
	cancel()
	wg.Wait()
	return err
}

func (a *Agent) sendLoop(ctx context.Context, stream nodev1.NodeControl_ConnectClient, outbox <-chan *nodev1.AgentMessage) {
	for {
		select {
		case <-ctx.Done():
			return
		case msg := <-outbox:
			if err := stream.Send(msg); err != nil {
				a.log.Warn("send to the panel failed", "error", err)
				return
			}
		}
	}
}

func (a *Agent) receiveLoop(ctx context.Context, stream nodev1.NodeControl_ConnectClient, outbox chan<- *nodev1.AgentMessage) error {
	for {
		msg, err := stream.Recv()
		if err != nil {
			if errors.Is(err, io.EOF) || ctx.Err() != nil {
				return nil
			}
			return err
		}
		switch payload := msg.Payload.(type) {
		case *nodev1.PanelMessage_ApplyConfig:
			a.applyConfig(ctx, payload.ApplyConfig, outbox)
		case *nodev1.PanelMessage_Command:
			a.runCommand(ctx, payload.Command, outbox)
		case *nodev1.PanelMessage_HelloAck:
			// Already handled during the handshake.
		}
	}
}

func (a *Agent) applyConfig(ctx context.Context, cmd *nodev1.ApplyConfig, outbox chan<- *nodev1.AgentMessage) {
	hash := cmd.GetConfigHash()
	if !cmd.GetForceRestart() && hash != "" && hash == a.xray.ConfigHash() && a.xray.Running() {
		a.log.Debug("configuration unchanged, skipping restart", "hash", hash)
		a.report(outbox, &nodev1.ApplyResult{
			RequestId: cmd.GetRequestId(), Ok: true, ConfigHash: hash, XrayVersion: a.xray.Version(),
		})
		return
	}

	a.log.Info("applying configuration from the panel", "hash", hash, "bytes", len(cmd.GetXrayConfig()))
	err := a.xray.Apply(ctx, cmd.GetXrayConfig(), hash)

	result := &nodev1.ApplyResult{
		RequestId:   cmd.GetRequestId(),
		Ok:          err == nil,
		ConfigHash:  a.xray.ConfigHash(),
		XrayVersion: a.xray.Version(),
	}
	a.mu.Lock()
	if err != nil {
		result.Error = err.Error()
		a.lastError = err.Error()
		a.log.Error("failed to apply configuration", "error", err)
	} else {
		a.lastError = ""
		a.log.Info("configuration applied", "hash", hash)
	}
	a.mu.Unlock()
	a.report(outbox, result)
}

func (a *Agent) runCommand(ctx context.Context, cmd *nodev1.Command, outbox chan<- *nodev1.AgentMessage) {
	switch cmd.GetKind() {
	case nodev1.Command_KIND_STOP_XRAY:
		a.log.Info("stopping xray at the panel's request")
		a.xray.Stop()
	case nodev1.Command_KIND_START_XRAY, nodev1.Command_KIND_RESTART_XRAY:
		a.log.Info("restarting xray at the panel's request")
		if err := a.xray.Start(ctx); err != nil {
			a.mu.Lock()
			a.lastError = err.Error()
			a.mu.Unlock()
			a.log.Error("restart failed", "error", err)
		}
	case nodev1.Command_KIND_FETCH_LOGS:
		lines := int(cmd.GetLogLines())
		if lines <= 0 {
			lines = 200
		}
		a.send(outbox, &nodev1.AgentMessage{Payload: &nodev1.AgentMessage_Logs{
			Logs: &nodev1.LogChunk{RequestId: cmd.GetRequestId(), Lines: a.xray.Logs(lines)},
		}})
	case nodev1.Command_KIND_PING:
		a.sendHeartbeat(outbox)
	}
}

func (a *Agent) telemetryLoop(ctx context.Context, outbox chan<- *nodev1.AgentMessage, heartbeat, usage time.Duration) {
	hbTicker := time.NewTicker(heartbeat)
	defer hbTicker.Stop()
	usageTicker := time.NewTicker(usage)
	defer usageTicker.Stop()

	a.sendHeartbeat(outbox)
	for {
		select {
		case <-ctx.Done():
			return
		case <-hbTicker.C:
			a.sendHeartbeat(outbox)
		case <-usageTicker.C:
			a.sendUsage(ctx, outbox)
		}
	}
}

func (a *Agent) sendHeartbeat(outbox chan<- *nodev1.AgentMessage) {
	snapshot := Snapshot()
	a.mu.Lock()
	message := a.lastError
	a.mu.Unlock()

	hb := &nodev1.Heartbeat{
		XrayRunning:     a.xray.Running(),
		XrayVersion:     a.xray.Version(),
		ConfigHash:      a.xray.ConfigHash(),
		CpuUsagePercent: snapshot.CPUUsage,
		UsedRamBytes:    snapshot.UsedRAMBytes,
		TotalRamBytes:   snapshot.TotalRAMBytes,
		LoadAvg_1:       snapshot.LoadAvg1,
		Message:         message,
	}
	if started := a.xray.StartedAt(); !started.IsZero() {
		hb.XrayStartedAtUnix = started.Unix()
	}
	a.send(outbox, &nodev1.AgentMessage{Payload: &nodev1.AgentMessage_Heartbeat{Heartbeat: hb}})
}

func (a *Agent) sendUsage(ctx context.Context, outbox chan<- *nodev1.AgentMessage) {
	if !a.xray.Running() {
		return
	}
	traffic, err := a.xray.CollectTraffic(ctx)
	if err != nil {
		a.log.Warn("could not read traffic counters", "error", err)
		return
	}

	report := &nodev1.UsageReport{CollectedAtUnix: time.Now().Unix()}
	for email, c := range traffic.Users {
		report.Users = append(report.Users, &nodev1.UserUsage{
			Email:         email,
			UplinkBytes:   c.Uplink,
			DownlinkBytes: c.Downlink,
			Online:        c.Uplink+c.Downlink > 0,
		})
	}
	for tag, c := range traffic.Inbounds {
		report.Inbounds = append(report.Inbounds, &nodev1.InboundUsage{
			Tag: tag, UplinkBytes: c.Uplink, DownlinkBytes: c.Downlink,
		})
	}
	if len(report.Users) == 0 && len(report.Inbounds) == 0 {
		return
	}
	a.send(outbox, &nodev1.AgentMessage{Payload: &nodev1.AgentMessage_Usage{Usage: report}})
}

func (a *Agent) report(outbox chan<- *nodev1.AgentMessage, result *nodev1.ApplyResult) {
	a.send(outbox, &nodev1.AgentMessage{Payload: &nodev1.AgentMessage_ApplyResult{ApplyResult: result}})
}

// send never blocks: dropping telemetry is preferable to stalling the agent
// when the panel is slow to read.
func (a *Agent) send(outbox chan<- *nodev1.AgentMessage, msg *nodev1.AgentMessage) {
	select {
	case outbox <- msg:
	default:
		a.log.Warn("dropped a message because the outbox is full")
	}
}
