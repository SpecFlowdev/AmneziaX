package httpapi

import (
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
)

type squadRequest struct {
	Name       string   `json:"name"`
	Info       string   `json:"info"`
	InboundIDs []string `json:"inboundUuids"`
}

func (a *API) listSquads(w http.ResponseWriter, r *http.Request) {
	squads, err := a.store.ListSquads(r.Context())
	if err != nil {
		a.storeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, squads)
}

func (a *API) getSquad(w http.ResponseWriter, r *http.Request) {
	squad, err := a.store.Squad(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		a.storeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, squad)
}

func (a *API) createSquad(w http.ResponseWriter, r *http.Request) {
	var req squadRequest
	if err := decode(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	if strings.TrimSpace(req.Name) == "" {
		writeErr(w, http.StatusBadRequest, "a squad name is required")
		return
	}
	squad, err := a.store.CreateSquad(r.Context(), strings.TrimSpace(req.Name), req.Info, req.InboundIDs)
	if err != nil {
		a.storeErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, squad)
}

func (a *API) updateSquad(w http.ResponseWriter, r *http.Request) {
	var req squadRequest
	if err := decode(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	if strings.TrimSpace(req.Name) == "" {
		writeErr(w, http.StatusBadRequest, "a squad name is required")
		return
	}
	squad, err := a.store.UpdateSquad(r.Context(), chi.URLParam(r, "id"), strings.TrimSpace(req.Name), req.Info, req.InboundIDs)
	if err != nil {
		a.storeErr(w, err)
		return
	}
	// Changing which inbounds a squad grants changes every member's access.
	a.hub.RequestSyncAll(r.Context())
	writeJSON(w, http.StatusOK, squad)
}

func (a *API) deleteSquad(w http.ResponseWriter, r *http.Request) {
	if err := a.store.DeleteSquad(r.Context(), chi.URLParam(r, "id")); err != nil {
		a.storeErr(w, err)
		return
	}
	a.hub.RequestSyncAll(r.Context())
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (a *API) squadAddAll(w http.ResponseWriter, r *http.Request) {
	affected, err := a.store.AddAllUsersToSquad(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		a.storeErr(w, err)
		return
	}
	a.hub.RequestSyncAll(r.Context())
	writeJSON(w, http.StatusOK, map[string]any{"affected": affected})
}

func (a *API) squadRemoveAll(w http.ResponseWriter, r *http.Request) {
	affected, err := a.store.RemoveAllUsersFromSquad(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		a.storeErr(w, err)
		return
	}
	a.hub.RequestSyncAll(r.Context())
	writeJSON(w, http.StatusOK, map[string]any{"affected": affected})
}
