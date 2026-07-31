package httpapi

import (
	"fmt"
	"net/http"
	"strings"

	nodev1 "github.com/SpecFlowdev/AmneziaX/gen/go/node/v1"
	"github.com/SpecFlowdev/AmneziaX/internal/auth"
	"github.com/SpecFlowdev/AmneziaX/internal/domain"
	"github.com/SpecFlowdev/AmneziaX/internal/storage/postgres"
	"github.com/go-chi/chi/v5"
)

type nodeRequest struct {
	Name              string                      `json:"name"`
	Address           string                      `json:"address"`
	CountryCode       string                      `json:"countryCode"`
	Description       string                      `json:"description"`
	IsDisabled        bool                        `json:"isDisabled"`
	ConfigProfileID   *string                     `json:"configProfileUuid"`
	ActiveInboundTags []string                    `json:"activeInboundTags"`
	Consumption       float64                     `json:"consumptionMultiplier"`
	TrafficLimitBytes int64                       `json:"trafficLimitBytes"`
	TrafficReset      domain.TrafficResetStrategy `json:"trafficResetStrategy"`
	NotifyPercent     int                         `json:"notifyPercent"`
	ViewPosition      int                         `json:"viewPosition"`
}

func (r nodeRequest) toInput() (postgres.NodeInput, error) {
	name := strings.TrimSpace(r.Name)
	if name == "" {
		return postgres.NodeInput{}, fmt.Errorf("a node name is required")
	}
	if r.Consumption <= 0 {
		r.Consumption = 1
	}
	if !r.TrafficReset.Valid() {
		r.TrafficReset = domain.ResetNever
	}
	if r.CountryCode == "" {
		r.CountryCode = "XX"
	}
	if r.ActiveInboundTags == nil {
		r.ActiveInboundTags = []string{}
	}
	if r.ConfigProfileID != nil && strings.TrimSpace(*r.ConfigProfileID) == "" {
		r.ConfigProfileID = nil
	}
	return postgres.NodeInput{
		Name:              name,
		Address:           strings.TrimSpace(r.Address),
		CountryCode:       strings.ToUpper(r.CountryCode),
		Description:       r.Description,
		IsDisabled:        r.IsDisabled,
		ConfigProfileID:   r.ConfigProfileID,
		ActiveInboundTags: r.ActiveInboundTags,
		Consumption:       r.Consumption,
		TrafficLimitBytes: r.TrafficLimitBytes,
		TrafficReset:      r.TrafficReset,
		NotifyPercent:     r.NotifyPercent,
		ViewPosition:      r.ViewPosition,
	}, nil
}

// nodeView adds the live connection state, which lives in the hub rather than
// in the database.
type nodeView struct {
	*domain.Node
	IsConnected bool `json:"isConnected"`
}

func (a *API) listNodes(w http.ResponseWriter, r *http.Request) {
	nodes, err := a.store.ListNodes(r.Context())
	if err != nil {
		a.storeErr(w, err)
		return
	}
	out := make([]nodeView, 0, len(nodes))
	for i := range nodes {
		out = append(out, nodeView{Node: &nodes[i], IsConnected: a.hub.Online(nodes[i].UUID)})
	}
	writeJSON(w, http.StatusOK, out)
}

func (a *API) getNode(w http.ResponseWriter, r *http.Request) {
	node, err := a.store.Node(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		a.storeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, nodeView{Node: node, IsConnected: a.hub.Online(node.UUID)})
}

// createNodeResponse returns the enrolment token exactly once, together with a
// ready-to-paste installation command.
type createNodeResponse struct {
	Node       *domain.Node `json:"node"`
	Token      string       `json:"token"`
	InstallCmd string       `json:"installCommand"`
}

func (a *API) createNode(w http.ResponseWriter, r *http.Request) {
	var req nodeRequest
	if err := decode(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	input, err := req.toInput()
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	token, digest, preview, err := auth.NewNodeToken()
	if err != nil {
		a.storeErr(w, err)
		return
	}
	node, err := a.store.CreateNode(r.Context(), input, digest, preview)
	if err != nil {
		a.storeErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, createNodeResponse{
		Node:       node,
		Token:      token,
		InstallCmd: a.installCommand(node.UUID, token),
	})
}

func (a *API) updateNode(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var req nodeRequest
	if err := decode(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	input, err := req.toInput()
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	node, err := a.store.UpdateNode(r.Context(), id, input)
	if err != nil {
		a.storeErr(w, err)
		return
	}
	a.hub.RequestSync(id)
	writeJSON(w, http.StatusOK, nodeView{Node: node, IsConnected: a.hub.Online(id)})
}

func (a *API) deleteNode(w http.ResponseWriter, r *http.Request) {
	if err := a.store.DeleteNode(r.Context(), chi.URLParam(r, "id")); err != nil {
		a.storeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (a *API) enableNode(w http.ResponseWriter, r *http.Request)  { a.setNodeDisabled(w, r, false) }
func (a *API) disableNode(w http.ResponseWriter, r *http.Request) { a.setNodeDisabled(w, r, true) }

func (a *API) setNodeDisabled(w http.ResponseWriter, r *http.Request, disabled bool) {
	id := chi.URLParam(r, "id")
	if err := a.store.SetNodeDisabled(r.Context(), id, disabled); err != nil {
		a.storeErr(w, err)
		return
	}
	if disabled {
		_ = a.hub.SendCommand(id, nodev1.Command_KIND_STOP_XRAY)
		_ = a.store.SetNodeHealth(r.Context(), id, domain.NodeHealthDisabled, "disabled by an administrator")
	} else {
		_ = a.store.SetNodeHealth(r.Context(), id, domain.NodeHealthConnecting, "enabled by an administrator")
		if err := a.hub.SyncNode(r.Context(), id, true); err != nil {
			a.log.Warn("enable node: sync failed", "node", id, "error", err)
		}
	}
	node, err := a.store.Node(r.Context(), id)
	if err != nil {
		a.storeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, nodeView{Node: node, IsConnected: a.hub.Online(id)})
}

func (a *API) restartNode(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := a.hub.SyncNode(r.Context(), id, true); err != nil {
		writeErr(w, http.StatusConflict, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (a *API) syncNode(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := a.hub.SyncNode(r.Context(), id, false); err != nil {
		writeErr(w, http.StatusConflict, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (a *API) rotateNodeToken(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	token, digest, preview, err := auth.NewNodeToken()
	if err != nil {
		a.storeErr(w, err)
		return
	}
	if err := a.store.RotateNodeToken(r.Context(), id, digest, preview); err != nil {
		a.storeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"token":          token,
		"installCommand": a.installCommand(id, token),
	})
}

func (a *API) resetNodeTraffic(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := a.store.ResetNodeTraffic(r.Context(), id); err != nil {
		a.storeErr(w, err)
		return
	}
	if err := a.hub.SyncNode(r.Context(), id, true); err != nil {
		a.log.Warn("reset node traffic: sync failed", "node", id, "error", err)
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (a *API) previewNodeConfig(w http.ResponseWriter, r *http.Request) {
	payload, err := a.hub.PreviewNodeConfig(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusConflict, err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_, _ = w.Write(payload)
}

func (a *API) nodeLogs(w http.ResponseWriter, r *http.Request) {
	lines, err := a.hub.FetchLogs(r.Context(), chi.URLParam(r, "id"), queryInt(r, "lines", 200))
	if err != nil {
		writeErr(w, http.StatusConflict, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"lines": lines})
}

// installCommand renders the single line an operator runs on a fresh server.
func (a *API) installCommand(nodeUUID, token string) string {
	host := a.cfg.GRPCPublicHost
	if host == "" {
		host = strings.TrimPrefix(strings.TrimPrefix(a.cfg.PublicURL, "https://"), "http://")
		if idx := strings.IndexAny(host, ":/"); idx >= 0 {
			host = host[:idx]
		}
	}
	return fmt.Sprintf(
		"bash <(curl -fsSL %s/install-node.sh) --panel %s:%d --uuid %s --token %s",
		a.cfg.PublicURL, host, a.cfg.GRPCPublicPort, nodeUUID, token)
}
