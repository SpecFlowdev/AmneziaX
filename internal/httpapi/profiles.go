package httpapi

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/SpecFlowdev/AmneziaX/internal/domain"
	"github.com/SpecFlowdev/AmneziaX/internal/hysteria"
	"github.com/SpecFlowdev/AmneziaX/internal/singbox"
	"github.com/SpecFlowdev/AmneziaX/internal/xray"
	"github.com/go-chi/chi/v5"
)

type profileRequest struct {
	Name string `json:"name"`
	// Kind names the engine this document is for. Empty means xray, so a
	// client written before this field existed keeps working.
	Kind   domain.ProfileKind `json:"kind"`
	Config json.RawMessage    `json:"config"`
}

func (r profileRequest) kind() domain.ProfileKind {
	if r.Kind == "" {
		return domain.ProfileXray
	}
	return r.Kind
}

// profileInbounds is what squads and hosts attach to.
//
// An xray document has real inbounds. A hysteria2 document has none — there is
// one server and one user list — so one is synthesised for it. That is what
// keeps the access model identical: a squad grants an inbound, whichever engine
// happens to be behind it, rather than hysteria needing its own parallel way of
// saying who may connect.
func profileInbounds(kind domain.ProfileKind, config json.RawMessage) ([]domain.ConfigProfileInbound, error) {
	if kind == domain.ProfileHysteria2 {
		port := 0
		var doc struct {
			Listen string `json:"listen"`
		}
		if err := json.Unmarshal(config, &doc); err == nil {
			if idx := strings.LastIndex(doc.Listen, ":"); idx >= 0 {
				port, _ = strconv.Atoi(doc.Listen[idx+1:])
			}
		}
		return []domain.ConfigProfileInbound{{
			Tag:      "hysteria2",
			Type:     "hysteria2",
			Network:  "udp",
			Security: "tls",
			Port:     port,
		}}, nil
	}
	if kind == domain.ProfileSingBox {
		ins, err := singbox.ParseInbounds(config)
		if err != nil {
			return nil, err
		}
		out := make([]domain.ConfigProfileInbound, 0, len(ins))
		for _, in := range ins {
			out = append(out, domain.ConfigProfileInbound{
				Tag: in.Tag, Type: in.Type, Network: "udp", Security: "tls", Port: in.Port,
			})
		}
		return out, nil
	}
	return xray.ParseInbounds(config)
}

// validateProfile sends the document to the rules of the engine that will
// actually run it. Validating a hysteria2 config against xray's rules would
// reject every valid one.
func validateProfile(kind domain.ProfileKind, config json.RawMessage) error {
	switch kind {
	case domain.ProfileHysteria2:
		return hysteria.Validate(config)
	case domain.ProfileSingBox:
		return singbox.Validate(config)
	case domain.ProfileXray, "":
		return xray.Validate(config)
	default:
		return errBadRequest("unknown profile kind " + string(kind))
	}
}

func (a *API) listProfiles(w http.ResponseWriter, r *http.Request) {
	profiles, err := a.store.ListProfiles(r.Context())
	if err != nil {
		a.storeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, profiles)
}

func (a *API) listInbounds(w http.ResponseWriter, r *http.Request) {
	inbounds, err := a.store.ProfileInbounds(r.Context(), r.URL.Query().Get("profileUuid"))
	if err != nil {
		a.storeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, inbounds)
}

func (a *API) getProfile(w http.ResponseWriter, r *http.Request) {
	profile, err := a.store.Profile(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		a.storeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, profile)
}

func (a *API) createProfile(w http.ResponseWriter, r *http.Request) {
	var req profileRequest
	if err := decode(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		writeErr(w, http.StatusBadRequest, "a profile name is required")
		return
	}
	// An omitted or null config means "give me something that works".
	if len(req.Config) == 0 || string(req.Config) == "null" {
		generated, err := xray.DefaultTemplate(xray.TemplateOptions{})
		if err != nil {
			a.storeErr(w, err)
			return
		}
		req.Config = generated
	}
	if err := validateProfile(req.kind(), req.Config); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	inbounds, err := profileInbounds(req.kind(), req.Config)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	profile, err := a.store.CreateProfile(r.Context(), req.Name, req.kind(), req.Config, inbounds)
	if err != nil {
		a.storeErr(w, err)
		return
	}
	a.store.LogEvent(r.Context(), domain.EventProfileUpdated, claimsOf(r).Username, profile.Name, "profile created", nil)
	writeJSON(w, http.StatusCreated, profile)
}

func (a *API) updateProfile(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var req profileRequest
	if err := decode(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	if strings.TrimSpace(req.Name) == "" {
		writeErr(w, http.StatusBadRequest, "a profile name is required")
		return
	}
	if err := validateProfile(req.kind(), req.Config); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	inbounds, err := profileInbounds(req.kind(), req.Config)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	profile, err := a.store.UpdateProfile(r.Context(), id, strings.TrimSpace(req.Name), req.kind(), req.Config, inbounds)
	if err != nil {
		a.storeErr(w, err)
		return
	}
	a.store.LogEvent(r.Context(), domain.EventProfileUpdated, claimsOf(r).Username, profile.Name, "profile updated", nil)
	a.hub.RequestSyncProfile(r.Context(), id)
	writeJSON(w, http.StatusOK, profile)
}

func (a *API) deleteProfile(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	nodes, err := a.store.NodesUsingProfile(r.Context(), id)
	if err != nil {
		a.storeErr(w, err)
		return
	}
	if len(nodes) > 0 {
		writeErr(w, http.StatusConflict, "this profile is still assigned to one or more nodes")
		return
	}
	if err := a.store.DeleteProfile(r.Context(), id); err != nil {
		a.storeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// realityKeys hands the UI a fresh x25519 pair and shortIds so an operator can
// fill in a REALITY inbound without leaving the browser.
func (a *API) realityKeys(w http.ResponseWriter, r *http.Request) {
	pair, err := xray.GenerateRealityKeys()
	if err != nil {
		a.storeErr(w, err)
		return
	}
	ids, err := xray.GenerateShortIDs(8)
	if err != nil {
		a.storeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"privateKey": pair.PrivateKey,
		"publicKey":  pair.PublicKey,
		"shortIds":   ids,
	})
}

type wireguardKeysRequest struct {
	// Given a private key, the answer is its public half. Left empty, a fresh
	// pair is generated instead.
	PrivateKey string `json:"privateKey"`
}

// wireguardKeys generates a server key pair, or derives the public half of one
// the operator already has. A WireGuard host must name the server's public key,
// and deriving it beats asking someone to keep the two halves straight by hand.
func (a *API) wireguardKeys(w http.ResponseWriter, r *http.Request) {
	var req wireguardKeysRequest
	if r.ContentLength > 0 {
		if err := decode(r, &req); err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
	}

	if key := strings.TrimSpace(req.PrivateKey); key != "" {
		pub, err := xray.WireGuardPublicKey(key)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"publicKey": pub})
		return
	}

	priv, pub, err := xray.NewWireGuardKey()
	if err != nil {
		a.storeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"privateKey": priv, "publicKey": pub})
}
