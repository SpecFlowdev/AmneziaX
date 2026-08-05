package xray

import (
	"encoding/json"
	"os"
	"os/exec"
	"strings"
	"testing"
)

// The point of this test is not that the JSON looks right — it is that the real
// xray binary accepts what the panel renders. A peer list that only satisfies
// our own unmarshalling is worth nothing on a node.
func TestRenderedWireGuardInboundIsAcceptedByXray(t *testing.T) {
	bin := os.Getenv("XRAY_BIN")
	if bin == "" {
		t.Skip("XRAY_BIN not set")
	}

	serverPriv, _, err := NewWireGuardKey()
	if err != nil {
		t.Fatal(err)
	}

	profile := json.RawMessage(`{
	  "inbounds": [{
	    "tag": "wg",
	    "port": 51820,
	    "protocol": "wireguard",
	    "settings": {"secretKey": "` + serverPriv + `"}
	  }],
	  "outbounds": [{"protocol": "freedom"}]
	}`)

	clients := []Client{}
	for i := int64(0); i < 3; i++ {
		_, pub, err := NewWireGuardKey()
		if err != nil {
			t.Fatal(err)
		}
		clients = append(clients, Client{
			Email:       "u" + string(rune('a'+i)),
			WGPublicKey: pub,
			WGAddress:   WireGuardAddress(i),
		})
	}
	// A user with no key at all must not break the inbound for the others.
	clients = append(clients, Client{Email: "keyless"})

	out, _, err := Render(profile, RenderOptions{
		ActiveTags:   []string{"wg"},
		ClientsByTag: map[string][]Client{"wg": clients},
	})
	if err != nil {
		t.Fatal(err)
	}

	var doc map[string]any
	if err := json.Unmarshal(out, &doc); err != nil {
		t.Fatal(err)
	}
	ins := doc["inbounds"].([]any)
	var peers []any
	for _, raw := range ins {
		in := raw.(map[string]any)
		if in["protocol"] == "wireguard" {
			peers = in["settings"].(map[string]any)["peers"].([]any)
		}
	}
	if len(peers) != 3 {
		t.Fatalf("got %d peers, want 3 (the keyless user must be skipped)", len(peers))
	}

	f, err := os.CreateTemp(t.TempDir(), "wg*.json")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.Write(out); err != nil {
		t.Fatal(err)
	}
	f.Close()

	cmd := exec.Command(bin, "run", "-test", "-c", f.Name())
	combined, err := cmd.CombinedOutput()
	if err != nil || !strings.Contains(string(combined), "Configuration OK") {
		t.Fatalf("xray rejected the rendered config: %v\n%s", err, combined)
	}
}
