package xray

import (
	"encoding/json"
	"strings"
	"testing"
)

func mustTemplate(t *testing.T) json.RawMessage {
	t.Helper()
	raw, err := DefaultTemplate(TemplateOptions{InboundTag: "vless-reality", Port: 443})
	if err != nil {
		t.Fatalf("DefaultTemplate: %v", err)
	}
	return raw
}

func TestValidateAcceptsTemplate(t *testing.T) {
	if err := Validate(mustTemplate(t)); err != nil {
		t.Fatalf("the default template must validate: %v", err)
	}
}

func TestValidateRejectsBadDocuments(t *testing.T) {
	cases := map[string]string{
		"not json":          `{`,
		"no inbounds":       `{"inbounds":[],"outbounds":[{"protocol":"freedom"}]}`,
		"missing tag":       `{"inbounds":[{"protocol":"vless","port":443}],"outbounds":[{"protocol":"freedom"}]}`,
		"duplicate tag":     `{"inbounds":[{"tag":"a","protocol":"vless"},{"tag":"a","protocol":"vless"}],"outbounds":[{"protocol":"freedom"}]}`,
		"reserved tag":      `{"inbounds":[{"tag":"amneziax-api","protocol":"vless"}],"outbounds":[{"protocol":"freedom"}]}`,
		"missing protocol":  `{"inbounds":[{"tag":"a","port":443}],"outbounds":[{"protocol":"freedom"}]}`,
		"missing outbounds": `{"inbounds":[{"tag":"a","protocol":"vless"}]}`,
	}
	for name, doc := range cases {
		t.Run(name, func(t *testing.T) {
			if err := Validate(json.RawMessage(doc)); err == nil {
				t.Fatalf("expected %s to be rejected", name)
			}
		})
	}
}

func TestParseInboundsExtractsStreamSettings(t *testing.T) {
	inbounds, err := ParseInbounds(mustTemplate(t))
	if err != nil {
		t.Fatalf("ParseInbounds: %v", err)
	}
	if len(inbounds) != 1 {
		t.Fatalf("got %d inbounds, want 1", len(inbounds))
	}
	got := inbounds[0]
	if got.Tag != "vless-reality" || got.Type != "vless" || got.Network != "tcp" ||
		got.Security != "reality" || got.Port != 443 {
		t.Fatalf("unexpected inbound summary: %+v", got)
	}
}

func TestRenderInjectsClientsAndStatsAPI(t *testing.T) {
	payload, hash, err := Render(mustTemplate(t), RenderOptions{
		ActiveTags: []string{"vless-reality"},
		ClientsByTag: map[string][]Client{
			"vless-reality": {
				{Email: "uuid.alice", VlessUUID: "11111111-1111-1111-1111-111111111111", Flow: "xtls-rprx-vision"},
			},
		},
	})
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if hash == "" {
		t.Fatal("Render must return a config hash")
	}

	var doc struct {
		Inbounds []struct {
			Tag      string `json:"tag"`
			Settings struct {
				Clients []struct {
					ID    string `json:"id"`
					Email string `json:"email"`
					Flow  string `json:"flow"`
				} `json:"clients"`
			} `json:"settings"`
		} `json:"inbounds"`
		Stats   map[string]any `json:"stats"`
		API     map[string]any `json:"api"`
		Routing struct {
			Rules []struct {
				InboundTag []string `json:"inboundTag"`
			} `json:"rules"`
		} `json:"routing"`
	}
	if err := json.Unmarshal(payload, &doc); err != nil {
		t.Fatalf("rendered config is not valid JSON: %v", err)
	}

	if doc.Stats == nil || doc.API == nil {
		t.Fatal("the stats API must be enabled so the agent can read counters")
	}
	if len(doc.Inbounds) != 2 || doc.Inbounds[0].Tag != apiInboundTag {
		t.Fatalf("expected the api inbound first, got %+v", doc.Inbounds)
	}
	if len(doc.Routing.Rules) == 0 || len(doc.Routing.Rules[0].InboundTag) == 0 ||
		doc.Routing.Rules[0].InboundTag[0] != apiInboundTag {
		t.Fatal("the api inbound must be routed internally by the first rule")
	}

	clients := doc.Inbounds[1].Settings.Clients
	if len(clients) != 1 {
		t.Fatalf("got %d clients, want 1", len(clients))
	}
	if clients[0].Email != "uuid.alice" || clients[0].Flow != "xtls-rprx-vision" {
		t.Fatalf("unexpected client entry: %+v", clients[0])
	}
}

func TestRenderIsStableAcrossCalls(t *testing.T) {
	template := mustTemplate(t)
	opts := RenderOptions{ActiveTags: []string{"vless-reality"}}

	_, first, err := Render(template, opts)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	_, second, err := Render(template, opts)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if first != second {
		t.Fatal("identical inputs must hash identically, otherwise nodes restart on every sync")
	}
}

func TestRenderDropsInactiveInbounds(t *testing.T) {
	doc := `{
		"inbounds": [
			{"tag":"keep","protocol":"vless","port":443,"settings":{"clients":[]}},
			{"tag":"drop","protocol":"vless","port":8443,"settings":{"clients":[]}}
		],
		"outbounds": [{"protocol":"freedom","tag":"direct"}]
	}`
	payload, _, err := Render(json.RawMessage(doc), RenderOptions{ActiveTags: []string{"keep"}})
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if strings.Contains(string(payload), `"drop"`) {
		t.Fatal("an inbound the node does not serve must not reach its config")
	}
	if !strings.Contains(string(payload), `"keep"`) {
		t.Fatal("the active inbound is missing from the rendered config")
	}
}

func TestRenderRejectsEmptySelection(t *testing.T) {
	_, _, err := Render(mustTemplate(t), RenderOptions{ActiveTags: []string{"nonexistent"}})
	if err == nil {
		t.Fatal("selecting only unknown tags must fail instead of shipping an empty config")
	}
}

func TestGenerateRealityKeysProducesDistinctPairs(t *testing.T) {
	a, err := GenerateRealityKeys()
	if err != nil {
		t.Fatalf("GenerateRealityKeys: %v", err)
	}
	b, err := GenerateRealityKeys()
	if err != nil {
		t.Fatalf("GenerateRealityKeys: %v", err)
	}
	if a.PrivateKey == b.PrivateKey || a.PublicKey == b.PublicKey {
		t.Fatal("each call must produce a fresh key pair")
	}
	if a.PrivateKey == a.PublicKey {
		t.Fatal("the private and public halves must differ")
	}
	// xray expects 32 raw bytes encoded with base64url, i.e. 43 characters.
	if len(a.PrivateKey) != 43 || len(a.PublicKey) != 43 {
		t.Fatalf("unexpected key length: %d / %d", len(a.PrivateKey), len(a.PublicKey))
	}
}

func TestGenerateShortIDsAreHexAndEvenLength(t *testing.T) {
	ids, err := GenerateShortIDs(8)
	if err != nil {
		t.Fatalf("GenerateShortIDs: %v", err)
	}
	if len(ids) != 8 {
		t.Fatalf("got %d ids, want 8", len(ids))
	}
	for _, id := range ids {
		if len(id)%2 != 0 || len(id) > 16 || len(id) == 0 {
			t.Fatalf("short id %q has an invalid length", id)
		}
		if strings.TrimLeft(id, "0123456789abcdef") != "" {
			t.Fatalf("short id %q is not hex", id)
		}
	}
}
