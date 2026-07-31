package httpapi

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/SpecFlowdev/AmneziaX/internal/domain"
	"github.com/SpecFlowdev/AmneziaX/internal/xray"
	"github.com/go-chi/chi/v5"
)

type profileRequest struct {
	Name   string          `json:"name"`
	Config json.RawMessage `json:"config"`
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
	if err := xray.Validate(req.Config); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	inbounds, err := xray.ParseInbounds(req.Config)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	profile, err := a.store.CreateProfile(r.Context(), req.Name, req.Config, inbounds)
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
	if err := xray.Validate(req.Config); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	inbounds, err := xray.ParseInbounds(req.Config)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	profile, err := a.store.UpdateProfile(r.Context(), id, strings.TrimSpace(req.Name), req.Config, inbounds)
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
