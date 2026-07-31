// Package hub owns the live connections to node agents and keeps the running
// xray configuration of every node in sync with the panel database.
package hub

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	nodev1 "github.com/SpecFlowdev/AmneziaX/gen/go/node/v1"
	"github.com/SpecFlowdev/AmneziaX/internal/config"
	"github.com/SpecFlowdev/AmneziaX/internal/domain"
	"github.com/SpecFlowdev/AmneziaX/internal/storage/postgres"
	"github.com/SpecFlowdev/AmneziaX/internal/xray"
	"github.com/google/uuid"
)

// session is one live agent stream.
type session struct {
	nodeID    string
	nodeName  string
	send      chan *nodev1.PanelMessage
	connected time.Time
	// done is closed once the session is retired, which releases both the
	// stream writer and any caller blocked in push.
	done      chan struct{}
	closeOnce sync.Once
	// logWaiters lets an API caller collect the log lines an agent streams back
	// in response to a fetch-logs command.
	logWaiters map[string]chan []string
	mu         sync.Mutex
}

func newSession(nodeID, nodeName string) *session {
	return &session{
		nodeID:     nodeID,
		nodeName:   nodeName,
		send:       make(chan *nodev1.PanelMessage, 32),
		connected:  time.Now(),
		done:       make(chan struct{}),
		logWaiters: map[string]chan []string{},
	}
}

func (s *session) push(msg *nodev1.PanelMessage) bool {
	select {
	case s.send <- msg:
		return true
	case <-s.done:
		return false
	default:
		return false
	}
}

func (s *session) close() { s.closeOnce.Do(func() { close(s.done) }) }

type Hub struct {
	nodev1.UnimplementedNodeControlServer

	store *postgres.Store
	cfg   *config.Panel
	log   *slog.Logger

	mu       sync.RWMutex
	sessions map[string]*session

	// pending coalesces sync requests so a bulk user edit results in one config
	// push per node instead of hundreds.
	pendingMu sync.Mutex
	pending   map[string]bool
	wake      chan struct{}
}

func New(store *postgres.Store, cfg *config.Panel, log *slog.Logger) *Hub {
	return &Hub{
		store:    store,
		cfg:      cfg,
		log:      log,
		sessions: map[string]*session{},
		pending:  map[string]bool{},
		wake:     make(chan struct{}, 1),
	}
}

// Online reports whether an agent stream is currently established.
func (h *Hub) Online(nodeID string) bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	_, ok := h.sessions[nodeID]
	return ok
}

// OnlineNodes returns the ids of every connected node.
func (h *Hub) OnlineNodes() []string {
	h.mu.RLock()
	defer h.mu.RUnlock()
	out := make([]string, 0, len(h.sessions))
	for id := range h.sessions {
		out = append(out, id)
	}
	return out
}

func (h *Hub) register(s *session) {
	h.mu.Lock()
	old := h.sessions[s.nodeID]
	h.sessions[s.nodeID] = s
	h.mu.Unlock()
	if old != nil {
		// A node reconnecting before the panel noticed the drop replaces the
		// stale session; retiring it releases the old stream writer.
		old.close()
	}
}

func (h *Hub) unregister(s *session) {
	h.mu.Lock()
	if cur, ok := h.sessions[s.nodeID]; ok && cur == s {
		delete(h.sessions, s.nodeID)
	}
	h.mu.Unlock()
	s.close()
}

func (h *Hub) sessionFor(nodeID string) *session {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.sessions[nodeID]
}

// ---------------------------------------------------------------- config sync

// RequestSync schedules a configuration push for one node.
func (h *Hub) RequestSync(nodeID string) {
	h.pendingMu.Lock()
	h.pending[nodeID] = true
	h.pendingMu.Unlock()
	select {
	case h.wake <- struct{}{}:
	default:
	}
}

// RequestSyncAll schedules a push for every node the panel knows about.
func (h *Hub) RequestSyncAll(ctx context.Context) {
	nodes, err := h.store.ListNodes(ctx)
	if err != nil {
		h.log.Error("sync all: list nodes", "error", err)
		return
	}
	for _, n := range nodes {
		h.RequestSync(n.UUID)
	}
}

// RequestSyncProfile schedules a push for every node bound to a profile.
func (h *Hub) RequestSyncProfile(ctx context.Context, profileID string) {
	nodes, err := h.store.NodesUsingProfile(ctx, profileID)
	if err != nil {
		h.log.Error("sync profile: list nodes", "error", err, "profile", profileID)
		return
	}
	for _, n := range nodes {
		h.RequestSync(n.UUID)
	}
}

// RequestSyncForUser pushes only to the nodes that can actually serve the user,
// derived from the profiles behind the user's squads.
func (h *Hub) RequestSyncForUser(ctx context.Context, userID string) {
	inboundIDs, err := h.store.UserInboundIDs(ctx, userID)
	if err != nil {
		h.log.Error("sync user: inbounds", "error", err, "user", userID)
		h.RequestSyncAll(ctx)
		return
	}
	profiles := map[string]bool{}
	for _, id := range inboundIDs {
		in, err := h.store.Inbound(ctx, id)
		if err != nil {
			continue
		}
		profiles[in.ConfigProfileID] = true
	}
	if len(profiles) == 0 {
		// The user lost every squad; the nodes that used to carry them still
		// need a push to drop the client entry.
		h.RequestSyncAll(ctx)
		return
	}
	for p := range profiles {
		h.RequestSyncProfile(ctx, p)
	}
}

// RunSyncLoop drains coalesced sync requests until the context is cancelled.
func (h *Hub) RunSyncLoop(ctx context.Context) {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-h.wake:
		case <-ticker.C:
		}

		h.pendingMu.Lock()
		batch := h.pending
		h.pending = map[string]bool{}
		h.pendingMu.Unlock()

		for nodeID := range batch {
			if err := h.SyncNode(ctx, nodeID, false); err != nil {
				h.log.Warn("node sync failed", "node", nodeID, "error", err)
			}
		}
	}
}

// SyncNode renders and pushes the configuration for a single node.
func (h *Hub) SyncNode(ctx context.Context, nodeID string, force bool) error {
	sess := h.sessionFor(nodeID)
	if sess == nil {
		return nil // Nothing to push to; the agent syncs on its next handshake.
	}
	node, err := h.store.Node(ctx, nodeID)
	if err != nil {
		return err
	}
	if node.IsDisabled {
		return h.stopNode(ctx, node)
	}
	if node.TrafficLimitBytes > 0 && node.TrafficUsedBytes >= node.TrafficLimitBytes {
		_ = h.store.SetNodeHealth(ctx, node.UUID, domain.NodeHealthTrafficLimit, "traffic limit reached")
		return h.stopNode(ctx, node)
	}

	payload, hash, err := h.BuildNodeConfig(ctx, node)
	if err != nil {
		_ = h.store.SetNodeHealth(ctx, node.UUID, domain.NodeHealthDegraded, err.Error())
		return err
	}
	if !force && hash == node.ConfigHash {
		return nil
	}

	msg := &nodev1.PanelMessage{Payload: &nodev1.PanelMessage_ApplyConfig{
		ApplyConfig: &nodev1.ApplyConfig{
			RequestId:    uuid.NewString(),
			XrayConfig:   payload,
			ConfigHash:   hash,
			ForceRestart: force,
		},
	}}
	if !sess.push(msg) {
		return fmt.Errorf("agent %s is not draining messages", node.Name)
	}
	h.store.LogEvent(ctx, domain.EventNodeConfigPushed, "system", node.Name,
		fmt.Sprintf("pushed configuration %s", short(hash)), map[string]any{"hash": hash})
	return nil
}

func (h *Hub) stopNode(ctx context.Context, node *domain.Node) error {
	sess := h.sessionFor(node.UUID)
	if sess == nil {
		return nil
	}
	sess.push(&nodev1.PanelMessage{Payload: &nodev1.PanelMessage_Command{
		Command: &nodev1.Command{RequestId: uuid.NewString(), Kind: nodev1.Command_KIND_STOP_XRAY},
	}})
	return nil
}

// BuildNodeConfig renders the exact xray document a node should run.
func (h *Hub) BuildNodeConfig(ctx context.Context, node *domain.Node) ([]byte, string, error) {
	if node.ConfigProfileID == nil || *node.ConfigProfileID == "" {
		return nil, "", fmt.Errorf("node has no configuration profile assigned")
	}
	profile, err := h.store.Profile(ctx, *node.ConfigProfileID)
	if err != nil {
		return nil, "", fmt.Errorf("load profile: %w", err)
	}

	tags := node.ActiveInboundTags
	if len(tags) == 0 {
		for _, in := range profile.Inbounds {
			tags = append(tags, in.Tag)
		}
	}
	if len(tags) == 0 {
		return nil, "", xray.ErrNoInbounds
	}

	provisioned, err := h.store.UsersForNode(ctx, profile.UUID, tags)
	if err != nil {
		return nil, "", fmt.Errorf("load users: %w", err)
	}

	// REALITY inbounds require the xtls-rprx-vision flow on the client entry;
	// the flow the operator configured on a host is the source of truth.
	flowByTag := map[string]string{}
	for _, in := range profile.Inbounds {
		if in.Security == "reality" && in.Type == "vless" && in.Network == "tcp" {
			flowByTag[in.Tag] = "xtls-rprx-vision"
		}
	}

	clients := map[string][]xray.Client{}
	for _, p := range provisioned {
		clients[p.InboundTag] = append(clients[p.InboundTag], xray.Client{
			Email:      p.UUID + "." + p.Username,
			VlessUUID:  p.VlessUUID,
			TrojanPass: p.TrojanPassword,
			SSPass:     p.SSPassword,
			Flow:       flowByTag[p.InboundTag],
		})
	}

	return xray.Render(profile.Config, xray.RenderOptions{
		ActiveTags:   tags,
		ClientsByTag: clients,
		APIListen:    "127.0.0.1",
		APIPort:      10085,
	})
}

// PreviewNodeConfig returns the rendered config as pretty JSON for the UI.
func (h *Hub) PreviewNodeConfig(ctx context.Context, nodeID string) (json.RawMessage, error) {
	node, err := h.store.Node(ctx, nodeID)
	if err != nil {
		return nil, err
	}
	payload, _, err := h.BuildNodeConfig(ctx, node)
	if err != nil {
		return nil, err
	}
	return payload, nil
}

// ---------------------------------------------------------------- commands

// SendCommand asks a connected agent to perform an action.
func (h *Hub) SendCommand(nodeID string, kind nodev1.Command_Kind) error {
	sess := h.sessionFor(nodeID)
	if sess == nil {
		return fmt.Errorf("node is not connected")
	}
	if !sess.push(&nodev1.PanelMessage{Payload: &nodev1.PanelMessage_Command{
		Command: &nodev1.Command{RequestId: uuid.NewString(), Kind: kind},
	}}) {
		return fmt.Errorf("node is not accepting commands")
	}
	return nil
}

// FetchLogs asks an agent for the tail of its xray log and waits for the reply.
func (h *Hub) FetchLogs(ctx context.Context, nodeID string, lines int) ([]string, error) {
	sess := h.sessionFor(nodeID)
	if sess == nil {
		return nil, fmt.Errorf("node is not connected")
	}
	if lines <= 0 || lines > 1000 {
		lines = 200
	}
	reqID := uuid.NewString()
	ch := make(chan []string, 1)

	sess.mu.Lock()
	if sess.logWaiters == nil {
		sess.logWaiters = map[string]chan []string{}
	}
	sess.logWaiters[reqID] = ch
	sess.mu.Unlock()

	defer func() {
		sess.mu.Lock()
		delete(sess.logWaiters, reqID)
		sess.mu.Unlock()
	}()

	if !sess.push(&nodev1.PanelMessage{Payload: &nodev1.PanelMessage_Command{
		Command: &nodev1.Command{RequestId: reqID, Kind: nodev1.Command_KIND_FETCH_LOGS, LogLines: uint32(lines)},
	}}) {
		return nil, fmt.Errorf("node is not accepting commands")
	}

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-time.After(10 * time.Second):
		return nil, fmt.Errorf("node did not answer in time")
	case out := <-ch:
		return out, nil
	}
}

func short(s string) string {
	if len(s) > 12 {
		return s[:12]
	}
	return s
}
