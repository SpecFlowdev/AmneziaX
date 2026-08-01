package httpapi

import (
	"net/http"
	"strings"
	"time"

	"github.com/SpecFlowdev/AmneziaX/internal/auth"
	"github.com/go-chi/chi/v5"
)

// spend answers "what does this fleet cost, and what has to be paid next".
func (a *API) spend(w http.ResponseWriter, r *http.Request) {
	settings, err := a.settings(r)
	if err != nil {
		a.storeErr(w, err)
		return
	}
	summary, err := a.store.Spend(r.Context(), settings.Currency)
	if err != nil {
		a.storeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, summary)
}

// ---------------------------------------------------------------- devices

func (a *API) userDevices(w http.ResponseWriter, r *http.Request) {
	devices, err := a.store.UserDevices(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		a.storeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, devices)
}

func (a *API) resetUserDevices(w http.ResponseWriter, r *http.Request) {
	if err := a.store.ResetDevices(r.Context(), chi.URLParam(r, "id")); err != nil {
		a.storeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (a *API) deleteUserDevice(w http.ResponseWriter, r *http.Request) {
	err := a.store.DeleteDevice(r.Context(), chi.URLParam(r, "id"), chi.URLParam(r, "hwid"))
	if err != nil {
		a.storeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// ---------------------------------------------------------------- api tokens

func (a *API) listTokens(w http.ResponseWriter, r *http.Request) {
	tokens, err := a.store.ListAPITokens(r.Context())
	if err != nil {
		a.storeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, tokens)
}

func (a *API) createToken(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name      string     `json:"name"`
		ExpiresAt *time.Time `json:"expiresAt"`
	}
	if err := decode(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		writeErr(w, http.StatusBadRequest, "a token name is required")
		return
	}

	token, digest, preview, err := auth.NewNodeToken()
	if err != nil {
		a.storeErr(w, err)
		return
	}
	created, err := a.store.CreateAPIToken(r.Context(), req.Name, digest, preview,
		claimsOf(r).Username, req.ExpiresAt)
	if err != nil {
		a.storeErr(w, err)
		return
	}
	// Shown once, exactly like a node enrolment token.
	writeJSON(w, http.StatusCreated, map[string]any{"token": token, "apiToken": created})
}

func (a *API) deleteToken(w http.ResponseWriter, r *http.Request) {
	if err := a.store.DeleteAPIToken(r.Context(), chi.URLParam(r, "id")); err != nil {
		a.storeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}
