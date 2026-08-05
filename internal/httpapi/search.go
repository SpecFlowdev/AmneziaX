package httpapi

import (
	"net/http"
	"strings"

	"github.com/SpecFlowdev/AmneziaX/internal/storage/postgres"
)

// SearchHit is one thing the operator can jump to. Kind and uuid are what the
// palette needs to build a route; label and hint are what it shows.
type SearchHit struct {
	Kind  string `json:"kind"`
	UUID  string `json:"uuid"`
	Label string `json:"label"`
	Hint  string `json:"hint,omitempty"`
}

// search answers the command palette. It is deliberately one endpoint over
// several tables rather than four calls from the browser: the palette runs on
// every keystroke, and four round trips per keystroke is how a search box comes
// to feel slow.
func (a *API) search(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if q == "" {
		writeJSON(w, http.StatusOK, []SearchHit{})
		return
	}
	// A per-kind cap rather than one overall, so a thousand matching users
	// cannot push the only matching node off the end of the list.
	const perKind = 5
	hits := make([]SearchHit, 0, perKind*4)

	if users, _, err := a.store.ListUsers(r.Context(), postgres.UserFilter{
		Search: q, Limit: perKind, SortBy: "username",
	}); err == nil {
		for i := range users {
			u := &users[i]
			hits = append(hits, SearchHit{
				Kind: "user", UUID: u.UUID, Label: u.Username, Hint: string(u.Status),
			})
		}
	}
	if nodes, err := a.store.ListNodes(r.Context()); err == nil {
		for i := range nodes {
			n := &nodes[i]
			if !matches(q, n.Name, n.Address, n.CountryCode) {
				continue
			}
			hits = append(hits, SearchHit{
				Kind: "node", UUID: n.UUID, Label: n.Name, Hint: n.Address,
			})
			if countKind(hits, "node") >= perKind {
				break
			}
		}
	}
	if hosts, err := a.store.ListHosts(r.Context()); err == nil {
		for i := range hosts {
			h := &hosts[i]
			if !matches(q, h.Remark, h.Address) {
				continue
			}
			hits = append(hits, SearchHit{
				Kind: "host", UUID: h.UUID, Label: h.Remark, Hint: h.Address,
			})
			if countKind(hits, "host") >= perKind {
				break
			}
		}
	}
	if squads, err := a.store.ListSquads(r.Context()); err == nil {
		for i := range squads {
			s := &squads[i]
			if !matches(q, s.Name) {
				continue
			}
			hits = append(hits, SearchHit{Kind: "squad", UUID: s.UUID, Label: s.Name})
			if countKind(hits, "squad") >= perKind {
				break
			}
		}
	}
	writeJSON(w, http.StatusOK, hits)
}

func matches(q string, fields ...string) bool {
	q = strings.ToLower(q)
	for _, f := range fields {
		if f != "" && strings.Contains(strings.ToLower(f), q) {
			return true
		}
	}
	return false
}

func countKind(hits []SearchHit, kind string) int {
	n := 0
	for i := range hits {
		if hits[i].Kind == kind {
			n++
		}
	}
	return n
}
