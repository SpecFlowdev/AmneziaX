package httpapi

import (
	"net/http"
	"runtime"
	"time"

	"github.com/SpecFlowdev/AmneziaX/internal/version"
)

var startedAt = time.Now()

func (a *API) overview(w http.ResponseWriter, r *http.Request) {
	o, err := a.store.Overview(r.Context())
	if err != nil {
		a.storeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"counters": o,
		"panel": map[string]any{
			"version":        version.Version,
			"commit":         version.Commit,
			"uptimeSeconds":  int64(time.Since(startedAt).Seconds()),
			"goVersion":      runtime.Version(),
			"connectedNodes": len(a.hub.OnlineNodes()),
		},
	})
}

func (a *API) trafficStats(w http.ResponseWriter, r *http.Request) {
	days := queryInt(r, "days", 7)
	if days <= 0 || days > 365 {
		days = 7
	}
	interval := "hour"
	if days > 2 {
		interval = "day"
	}
	since := time.Now().Add(-time.Duration(days) * 24 * time.Hour)

	total, err := a.store.TotalTrafficSeries(r.Context(), since, interval)
	if err != nil {
		a.storeErr(w, err)
		return
	}
	perNode, err := a.store.NodeTrafficSeries(r.Context(), since, interval)
	if err != nil {
		a.storeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"interval": interval,
		"since":    since,
		"total":    total,
		"nodes":    perNode,
	})
}

func (a *API) topUsers(w http.ResponseWriter, r *http.Request) {
	days := queryInt(r, "days", 7)
	if days <= 0 || days > 365 {
		days = 7
	}
	limit := queryInt(r, "limit", 10)
	if limit <= 0 || limit > 100 {
		limit = 10
	}
	items, err := a.store.TopUsers(r.Context(), time.Now().Add(-time.Duration(days)*24*time.Hour), limit)
	if err != nil {
		a.storeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, items)
}

func (a *API) events(w http.ResponseWriter, r *http.Request) {
	items, err := a.store.ListEvents(r.Context(), queryInt(r, "limit", 100), r.URL.Query().Get("kind"))
	if err != nil {
		a.storeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, items)
}

// attention answers "what needs me right now" in one request, so the operator
// does not have to open four pages to find out that nothing does.
func (a *API) attention(w http.ResponseWriter, r *http.Request) {
	settings, err := a.settings(r)
	if err != nil {
		a.storeErr(w, err)
		return
	}
	out, err := a.store.Attention(r.Context(), settings.WarnExpiryDays, settings.WarnQuotaPercent)
	if err != nil {
		a.storeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}
