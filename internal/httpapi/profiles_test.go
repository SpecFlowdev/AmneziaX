package httpapi

import "testing"

// The domain arrives from a browser and is placed inside a document the panel
// generates, so what it may contain is worth pinning down. A hostname is a
// small alphabet; anything outside it is dropped whole rather than stripped
// character by character, because a half-cleaned name is a name nobody asked
// for and the starter is better off with its own placeholder.
func TestSanitiseHostname(t *testing.T) {
	for _, tc := range []struct {
		name string
		in   string
		want string
	}{
		{name: "plain hostname", in: "vpn.example.org", want: "vpn.example.org"},
		{name: "trimmed and lowered", in: "  VPN.Example.ORG ", want: "vpn.example.org"},
		{name: "dashes are legal", in: "edge-1.example.org", want: "edge-1.example.org"},
		{name: "empty stays empty", in: "", want: ""},

		// Everything below would end up inside the generated JSON.
		{name: "quote", in: `example.org","x":"y`, want: ""},
		{name: "space", in: "example.org evil.org", want: ""},
		{name: "newline", in: "example.org\nlisten: :1", want: ""},
		{name: "url rather than a host", in: "https://example.org/", want: ""},
		{name: "longer than a domain name may be", in: string(make([]byte, 300)), want: ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := sanitiseHostname(tc.in); got != tc.want {
				t.Fatalf("sanitiseHostname(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}
