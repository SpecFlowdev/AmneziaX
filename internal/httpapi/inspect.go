package httpapi

import "net/http"

// The inspectors answer questions that span every subscriber at once — which
// device is this, who is polling a link that no longer resolves — and neither
// is answerable from a single user's page.

func (a *API) inspectDevices(w http.ResponseWriter, r *http.Request) {
	items, err := a.store.AllDevices(r.Context(),
		r.URL.Query().Get("q"), queryInt(r, "limit", 200))
	if err != nil {
		a.storeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, items)
}

func (a *API) inspectSubscriptionRequests(w http.ResponseWriter, r *http.Request) {
	items, err := a.store.SubscriptionRequests(r.Context(),
		r.URL.Query().Get("user"),
		r.URL.Query().Get("failed") == "1",
		queryInt(r, "limit", 200))
	if err != nil {
		a.storeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, items)
}
