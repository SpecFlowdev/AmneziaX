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

	"bytes"
	"crypto/sha256"
	"encoding/hex"
	nodev1 "github.com/SpecFlowdev/AmneziaX/gen/go/node/v1"
	"github.com/SpecFlowdev/AmneziaX/internal/config"

	"github.com/SpecFlowdev/AmneziaX/internal/domain"
	"github.com/SpecFlowdev/AmneziaX/internal/hysteria"
	"github.com/SpecFlowdev/AmneziaX/internal/singbox"
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

	// A node whose profile is not an xray document has no xray config to send.
	// Rendering one would fail on "no inbounds", which is true and useless.
	var payload []byte
	var hash string
	xrayNode, err := h.nodeRunsXray(ctx, node)
	if err != nil {
		_ = h.store.SetNodeHealth(ctx, node.UUID, domain.NodeHealthDegraded, err.Error())
		return err
	}
	if xrayNode {
		payload, hash, err = h.BuildNodeConfig(ctx, node)
		if err != nil {
			_ = h.store.SetNodeHealth(ctx, node.UUID, domain.NodeHealthDegraded, err.Error())
			return err
		}
	}

	// Any engine beyond xray this node should also run. A failure to render one
	// is reported but does not block the xray push: a node serving most of what
	// it should beats a node serving none of it.
	cores, coreHash, cerr := h.buildExtraCores(ctx, node)
	if cerr != nil {
		h.log.Warn("could not render an additional core", "node", node.Name, "error", cerr)
	}
	// The hash the node compares against has to cover every core, otherwise a
	// change to the hysteria document alone would look like "nothing changed".
	if coreHash != "" {
		sum := sha256.Sum256([]byte(hash + coreHash))
		hash = hex.EncodeToString(sum[:])
	}

	if !force && hash == node.ConfigHash {
		return nil
	}

	msg := &nodev1.PanelMessage{Payload: &nodev1.PanelMessage_ApplyConfig{
		ApplyConfig: &nodev1.ApplyConfig{
			RequestId:    uuid.NewString(),
			XrayConfig:   payload,
			Cores:        cores,
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
			Email:       p.UUID + "." + p.Username,
			VlessUUID:   p.VlessUUID,
			TrojanPass:  p.TrojanPassword,
			SSPass:      p.SSPassword,
			Flow:        flowByTag[p.InboundTag],
			WGPublicKey: p.WGPublicKey,
			WGAddress:   xray.WireGuardAddress(p.WGIndex),
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

	// A node whose profile is a hysteria2 document has no xray config to show.
	// Showing the error from rendering one would be technically true and
	// completely unhelpful — the operator wants to see what the node runs.
	xrayNode, err := h.nodeRunsXray(ctx, node)
	if err != nil {
		return nil, err
	}
	if !xrayNode {
		cores, _, err := h.buildExtraCores(ctx, node)
		if err != nil {
			return nil, err
		}
		if len(cores) == 0 {
			return nil, fmt.Errorf("nothing to render for this node")
		}
		var pretty bytes.Buffer
		if err := json.Indent(&pretty, cores[0].GetConfig(), "", "  "); err != nil {
			return cores[0].GetConfig(), nil
		}
		return pretty.Bytes(), nil
	}

	payload, _, err := h.BuildNodeConfig(ctx, node)
	if err != nil {
		return nil, err
	}
	var pretty bytes.Buffer
	if err := json.Indent(&pretty, payload, "", "  "); err != nil {
		return payload, nil
	}
	return pretty.Bytes(), nil
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

// buildExtraCores renders every engine this node runs besides xray.
//
// A node is bound to one profile, so today that means: if the profile is a
// hysteria2 document, the node runs hysteria2 instead of xray. The list shape
// is what makes running both at once a change of data rather than a change of
// contract.
func (h *Hub) buildExtraCores(ctx context.Context, node *domain.Node) ([]*nodev1.CoreConfig, string, error) {
	if node.ConfigProfileID == nil || *node.ConfigProfileID == "" {
		return nil, "", nil
	}
	profile, err := h.store.Profile(ctx, *node.ConfigProfileID)
	if err != nil {
		return nil, "", err
	}
	switch profile.Kind {
	case domain.ProfileHysteria2:
	case domain.ProfileSingBox:
		return h.buildSingBoxCore(ctx, node, profile)
	default:
		return nil, "", nil
	}

	users, err := h.store.UsersForProfile(ctx, profile.UUID)
	if err != nil {
		return nil, "", err
	}
	list := make([]hysteria.User, 0, len(users))
	for i := range users {
		u := &users[i]
		list = append(list, hysteria.User{
			Name: hysteria.AuthKey(u.UUID, u.Username),
			// Hysteria authenticates with a single password, and the trojan
			// one is already a per-user secret that rotates when the user is
			// revoked — so revoking still cuts hysteria access off too.
			Password: u.TrojanPassword,
		})
	}

	payload, hash, err := hysteria.Render(profile.Config, hysteria.RenderOptions{Users: list})
	if err != nil {
		return nil, "", err
	}
	return []*nodev1.CoreConfig{{
		Kind:   string(domain.ProfileHysteria2),
		Config: payload,
		Hash:   hash,
	}}, hash, nil
}

// nodeRunsXray reports whether this node's profile is an xray document. Only
// the profile decides — a node does not choose an engine, it runs whatever its
// profile is written for.
func (h *Hub) nodeRunsXray(ctx context.Context, node *domain.Node) (bool, error) {
	if node.ConfigProfileID == nil || *node.ConfigProfileID == "" {
		return false, fmt.Errorf("node has no configuration profile assigned")
	}
	profile, err := h.store.Profile(ctx, *node.ConfigProfileID)
	if err != nil {
		return false, err
	}
	return profile.Kind != domain.ProfileHysteria2 && profile.Kind != domain.ProfileSingBox, nil
}

// buildSingBoxCore renders the sing-box document for a node, with the users
// each of its inbounds should serve.
//
// Unlike hysteria, sing-box has real inbounds, so access works exactly as it
// does for xray: a squad grants an inbound, and only the users reachable
// through it are written into that inbound.
func (h *Hub) buildSingBoxCore(ctx context.Context, node *domain.Node, profile *domain.ConfigProfile) ([]*nodev1.CoreConfig, string, error) {
	tags := node.ActiveInboundTags
	if len(tags) == 0 {
		for _, in := range profile.Inbounds {
			tags = append(tags, in.Tag)
		}
	}

	provisioned, err := h.store.UsersForNode(ctx, profile.UUID, tags)
	if err != nil {
		return nil, "", err
	}
	byTag := map[string][]singbox.User{}
	for _, p := range provisioned {
		byTag[p.InboundTag] = append(byTag[p.InboundTag], singbox.User{
			Name: p.UUID + "." + p.Username,
			UUID: p.VlessUUID,
			// The trojan password doubles as the hysteria2, tuic and trojan
			// secret, so revoking a user still cuts off every one of them.
			Password: p.TrojanPassword,
		})
	}

	payload, hash, err := singbox.Render(profile.Config, singbox.RenderOptions{
		UsersByTag: byTag,
		ActiveTags: tags,
	})
	if err != nil {
		return nil, "", err
	}
	return []*nodev1.CoreConfig{{
		Kind: string(domain.ProfileSingBox), Config: payload, Hash: hash,
	}}, hash, nil
}
