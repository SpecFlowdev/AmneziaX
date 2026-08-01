package httpapi

import (
	"net/http/httptest"
	"testing"
)

// /s/<uuid> is the only link an operator hands out, so this one predicate
// decides whether a subscriber sees their page or their client gets a working
// configuration. Guessing wrong in either direction is silent: the browser
// downloads a file nobody can read, or the app imports HTML and reports an
// empty subscription.
func TestWantsHTML(t *testing.T) {
	for _, tc := range []struct {
		name   string
		accept string
		query  string
		want   bool
	}{
		{name: "chrome navigation", accept: "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,*/*;q=0.8", want: true},
		{name: "safari navigation", accept: "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8", want: true},
		{name: "accept with parameters", accept: "text/html; charset=utf-8", want: true},

		// Client apps ask for anything, or say nothing at all. Neither may be
		// answered with a page.
		{name: "client sending */*", accept: "*/*", want: false},
		{name: "client sending nothing", accept: "", want: false},
		{name: "client asking for plain text", accept: "text/plain", want: false},

		// A browser that merely mentions html far down its list is still a
		// browser; a client that never mentions it is still a client.
		{name: "html not first", accept: "application/json, text/html;q=0.1", want: true},

		// An explicit format is the operator's decision and outranks sniffing,
		// in both directions.
		{name: "format=clash from a browser", accept: "text/html", query: "format=clash", want: false},
		{name: "format=page from a client", accept: "*/*", query: "format=page", want: true},
		{name: "format=html from a client", accept: "*/*", query: "format=html", want: true},
		{name: "unknown format from a browser", accept: "text/html", query: "format=nonsense", want: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest("GET", "/s/token?"+tc.query, nil)
			// httptest sets no Accept of its own; an empty string must stay empty.
			if tc.accept != "" {
				r.Header.Set("Accept", tc.accept)
			}
			if got := wantsHTML(r); got != tc.want {
				t.Fatalf("wantsHTML(accept=%q, %q) = %v, want %v", tc.accept, tc.query, got, tc.want)
			}
		})
	}
}
