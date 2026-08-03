package httpapi

import (
	"errors"
	"net/http"
	"strings"

	"github.com/SpecFlowdev/AmneziaX/internal/storage/postgres"
	"github.com/SpecFlowdev/AmneziaX/internal/subscription"
	"github.com/go-chi/chi/v5"
)

func normaliseRule(r postgres.ResponseRule) (postgres.ResponseRule, error) {
	r.Name = strings.TrimSpace(r.Name)
	r.MatchUA = strings.TrimSpace(r.MatchUA)
	if r.MatchUA == "" {
		return r, errors.New("the rule needs something to match on")
	}
	if len(r.MatchUA) > 200 {
		return r, errors.New("the match text is too long")
	}
	r.Format = strings.ToLower(strings.TrimSpace(r.Format))
	if !subscription.Format(r.Format).Valid() {
		return r, errors.New("unknown format")
	}
	if r.Priority <= 0 {
		r.Priority = 100
	}
	return r, nil
}

func (a *API) listRules(w http.ResponseWriter, r *http.Request) {
	items, err := a.store.ListRules(r.Context())
	if err != nil {
		a.storeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, items)
}

func (a *API) createRule(w http.ResponseWriter, r *http.Request) {
	var req postgres.ResponseRule
	if err := decode(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	rec, err := normaliseRule(req)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	created, err := a.store.CreateRule(r.Context(), rec)
	if err != nil {
		a.storeErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, created)
}

func (a *API) updateRule(w http.ResponseWriter, r *http.Request) {
	var req postgres.ResponseRule
	if err := decode(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	rec, err := normaliseRule(req)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	updated, err := a.store.UpdateRule(r.Context(), chi.URLParam(r, "id"), rec)
	if err != nil {
		a.storeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

func (a *API) deleteRule(w http.ResponseWriter, r *http.Request) {
	if err := a.store.DeleteRule(r.Context(), chi.URLParam(r, "id")); err != nil {
		a.storeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// testRule answers "what would this client actually get", which is the only
// question an operator has while writing one.
func (a *API) testRule(w http.ResponseWriter, r *http.Request) {
	ua := r.URL.Query().Get("ua")
	probe := r.Clone(r.Context())
	probe.Header.Set("User-Agent", ua)
	probe.URL.RawQuery = ""

	writeJSON(w, http.StatusOK, map[string]any{
		"userAgent": ua,
		"format":    string(a.formatFor(probe, false)),
	})
}
