package hub

import (
	"context"
	"errors"
	"io"
	"strings"
	"time"

	nodev1 "github.com/SpecFlowdev/AmneziaX/gen/go/node/v1"
	"github.com/SpecFlowdev/AmneziaX/internal/auth"
	"github.com/SpecFlowdev/AmneziaX/internal/domain"
	"github.com/SpecFlowdev/AmneziaX/internal/storage/postgres"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Connect implements the agent-facing control stream. The agent dials in,
// authenticates with its enrolment token and then keeps the stream open for the
// lifetime of the process.
func (h *Hub) Connect(stream nodev1.NodeControl_ConnectServer) error {
	ctx := stream.Context()

	first, err := stream.Recv()
	if err != nil {
		return err
	}
	hello := first.GetHello()
	if hello == nil {
		return status.Error(codes.InvalidArgument, "first message must be a hello")
	}

	node, err := h.authenticate(ctx, hello)
	if err != nil {
		_ = stream.Send(&nodev1.PanelMessage{Payload: &nodev1.PanelMessage_HelloAck{
			HelloAck: &nodev1.HelloAck{Accepted: false, Error: err.Error()},
		}})
		return status.Error(codes.Unauthenticated, err.Error())
	}

	sys := hello.GetSystem()
	info := postgres.NodeConnectedInfo{
		AgentVersion: hello.GetAgentVersion(),
		XrayVersion:  hello.GetXrayVersion(),
		ConfigHash:   hello.GetConfigHash(),
	}
	if sys != nil {
		info.Hostname = sys.GetHostname()
		info.OS = sys.GetOs()
		info.Arch = sys.GetArch()
		info.Kernel = sys.GetKernel()
		info.CPUCount = int(sys.GetCpuCount())
		info.CPUModel = sys.GetCpuModel()
		info.TotalRAMBytes = int64(sys.GetTotalRamBytes())
	}
	if err := h.store.MarkNodeConnected(ctx, node.UUID, info); err != nil {
		h.log.Error("mark node connected", "node", node.Name, "error", err)
	}
	h.store.LogEvent(ctx, domain.EventNodeConnected, "node", node.Name,
		"agent "+hello.GetAgentVersion()+" connected", map[string]any{"xray": hello.GetXrayVersion()})
	h.log.Info("node connected", "node", node.Name, "agent", hello.GetAgentVersion(), "xray", hello.GetXrayVersion())

	sess := newSession(node.UUID, node.Name)
	h.register(sess)
	defer func() {
		h.unregister(sess)
		disconnectCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := h.store.MarkNodeDisconnected(disconnectCtx, node.UUID, "agent disconnected"); err != nil {
			h.log.Error("mark node disconnected", "node", node.Name, "error", err)
		}
		h.store.LogEvent(disconnectCtx, domain.EventNodeDisconnected, "node", node.Name, "agent disconnected", nil)
		h.log.Info("node disconnected", "node", node.Name)
	}()

	if err := stream.Send(&nodev1.PanelMessage{Payload: &nodev1.PanelMessage_HelloAck{
		HelloAck: &nodev1.HelloAck{
			Accepted:                 true,
			NodeName:                 node.Name,
			HeartbeatIntervalSeconds: uint32(h.cfg.HeartbeatInterval.Seconds()),
			UsageIntervalSeconds:     uint32(h.cfg.UsageInterval.Seconds()),
		},
	}}); err != nil {
		return err
	}

	// Push the current configuration right away so a restarted agent converges
	// without waiting for the next edit.
	h.RequestSync(node.UUID)

	go func() {
		for {
			select {
			case <-sess.done:
				return
			case <-ctx.Done():
				return
			case msg := <-sess.send:
				if err := stream.Send(msg); err != nil {
					h.log.Warn("send to node failed", "node", sess.nodeName, "error", err)
					sess.close()
					return
				}
			}
		}
	}()

	for {
		msg, err := stream.Recv()
		if err != nil {
			if errors.Is(err, io.EOF) || status.Code(err) == codes.Canceled {
				return nil
			}
			return err
		}
		h.handleAgentMessage(ctx, node.UUID, sess, msg)
	}
}

func (h *Hub) authenticate(ctx context.Context, hello *nodev1.Hello) (*domain.Node, error) {
	nodeID := strings.TrimSpace(hello.GetNodeUuid())
	if nodeID == "" || hello.GetToken() == "" {
		return nil, errors.New("node uuid and token are required")
	}
	node, err := h.store.Node(ctx, nodeID)
	if err != nil {
		if errors.Is(err, postgres.ErrNotFound) {
			return nil, errors.New("unknown node")
		}
		return nil, errors.New("node lookup failed")
	}
	if !auth.TokenMatches(node.TokenHash, hello.GetToken()) {
		h.store.LogEvent(ctx, domain.EventNodeError, "node", node.Name, "rejected an agent with an invalid token", nil)
		return nil, errors.New("invalid node token")
	}
	return node, nil
}

func (h *Hub) handleAgentMessage(ctx context.Context, nodeID string, sess *session, msg *nodev1.AgentMessage) {
	switch payload := msg.Payload.(type) {
	case *nodev1.AgentMessage_Heartbeat:
		h.handleHeartbeat(ctx, nodeID, payload.Heartbeat)
	case *nodev1.AgentMessage_ApplyResult:
		h.handleApplyResult(ctx, nodeID, sess, payload.ApplyResult)
	case *nodev1.AgentMessage_Usage:
		h.handleUsage(ctx, nodeID, payload.Usage)
	case *nodev1.AgentMessage_Logs:
		sess.mu.Lock()
		ch := sess.logWaiters[payload.Logs.GetRequestId()]
		sess.mu.Unlock()
		if ch != nil {
			select {
			case ch <- payload.Logs.GetLines():
			default:
			}
		}
	case *nodev1.AgentMessage_Hello:
		// A duplicate hello on an established stream is ignored.
	}
}

func (h *Hub) handleHeartbeat(ctx context.Context, nodeID string, hb *nodev1.Heartbeat) {
	health := domain.NodeHealthOnline
	if !hb.GetXrayRunning() {
		health = domain.NodeHealthDegraded
	}
	var startedAt *time.Time
	if ts := hb.GetXrayStartedAtUnix(); ts > 0 {
		t := time.Unix(ts, 0)
		startedAt = &t
	}
	err := h.store.ApplyNodeHeartbeat(ctx, nodeID, postgres.NodeHeartbeat{
		Health:        health,
		XrayRunning:   hb.GetXrayRunning(),
		XrayStartedAt: startedAt,
		XrayVersion:   hb.GetXrayVersion(),
		ConfigHash:    hb.GetConfigHash(),
		CPUUsage:      hb.GetCpuUsagePercent(),
		UsedRAMBytes:  int64(hb.GetUsedRamBytes()),
		TotalRAMBytes: int64(hb.GetTotalRamBytes()),
		LoadAvg1:      hb.GetLoadAvg_1(),
		OnlineUsers:   int(hb.GetOnlineUsers()),
		Message:       hb.GetMessage(),
	})
	if err != nil {
		h.log.Error("apply heartbeat", "node", nodeID, "error", err)
	}

	// The node row only ever holds the latest reading, which answers "is it
	// busy now" but not "has it been busy for an hour". Keep the sample too;
	// the store buckets it by minute so a chatty agent cannot flood the table.
	if err := h.store.RecordNodeMetric(ctx, nodeID, domain.NodeMetric{
		CPUPercent:  hb.GetCpuUsagePercent(),
		UsedRAM:     int64(hb.GetUsedRamBytes()),
		TotalRAM:    int64(hb.GetTotalRamBytes()),
		LoadAvg1:    hb.GetLoadAvg_1(),
		OnlineUsers: int(hb.GetOnlineUsers()),
	}); err != nil {
		// A missing sample costs a gap in a chart, never the node's liveness.
		h.log.Debug("record node metric", "node", nodeID, "error", err)
	}
}

func (h *Hub) handleApplyResult(ctx context.Context, nodeID string, sess *session, res *nodev1.ApplyResult) {
	if res.GetOk() {
		if err := h.store.SetNodeConfigHash(ctx, nodeID, res.GetConfigHash()); err != nil {
			h.log.Error("store config hash", "node", nodeID, "error", err)
		}
		h.log.Info("node applied config", "node", sess.nodeName, "hash", short(res.GetConfigHash()))
		return
	}
	h.log.Error("node failed to apply config", "node", sess.nodeName, "error", res.GetError())
	_ = h.store.SetNodeHealth(ctx, nodeID, domain.NodeHealthDegraded, res.GetError())
	h.store.LogEvent(ctx, domain.EventNodeError, "node", sess.nodeName,
		"failed to apply configuration: "+res.GetError(), nil)
}

// Hub implements the generated node control service.
var _ nodev1.NodeControlServer = (*Hub)(nil)
