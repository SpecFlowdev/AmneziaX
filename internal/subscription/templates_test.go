package subscription

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/SpecFlowdev/AmneziaX/internal/domain"
)

func templateBundle() Bundle {
	h := domain.Host{
		InboundType: "vless", InboundTag: "vless-in", Remark: "DE",
		Address: "de.example.com", Port: 443,
		Security: "reality", SNI: "www.microsoft.com",
		PublicKey: "W5eMFnk7jEC_TX130qOipwz-pd66UcWqFSlYvZwMWjo", ShortID: "beef",
	}
	return Bundle{User: testUser(), Hosts: []domain.Host{h}, Title: "AmneziaX"}
}

// An empty template must leave the built-in document untouched — this is the
// path every existing deployment is on, and it has to stay byte-identical.
func TestRenderWithEmptyTemplateIsUnchanged(t *testing.T) {
	b := templateBundle()
	for _, f := range []Format{FormatClash, FormatSingBox, FormatBase64, FormatJSON} {
		if got, want := RenderWith(b, f, Templates{}), Render(b, f); got != want {
			t.Errorf("%s: empty template changed the output", f)
		}
	}
}

func TestClashTemplateSubstitution(t *testing.T) {
	out := RenderWith(templateBundle(), FormatClash, Templates{
		Clash: "# {{TITLE}}\nproxies:\n{{PROXIES}}\ngroups: [{{NAMES}}]\n",
	})

	if !strings.Contains(out, "# AmneziaX") {
		t.Errorf("title not substituted:\n%s", out)
	}
	// The subscriber's actual server has to arrive, not just the scaffolding.
	if !strings.Contains(out, "de.example.com") || !strings.Contains(out, "reality-opts") {
		t.Errorf("proxy entry missing:\n%s", out)
	}
	if !strings.Contains(out, `groups: ["DE"]`) {
		t.Errorf("names not substituted:\n%s", out)
	}
	for _, ph := range []string{PlaceholderProxies, PlaceholderNames, PlaceholderTitle} {
		if strings.Contains(out, ph) {
			t.Errorf("placeholder %s survived into the output", ph)
		}
	}
}

func TestSingBoxTemplateProducesValidJSON(t *testing.T) {
	out := RenderWith(templateBundle(), FormatSingBox, Templates{
		SingBox: `{"outbounds":[{{OUTBOUNDS}}],"selector":[{{TAGS}}]}`,
	})

	var doc struct {
		Outbounds []map[string]any `json:"outbounds"`
		Selector  []string         `json:"selector"`
	}
	// The whole point of splicing rather than string-pasting: what comes out
	// still has to parse, or the client rejects the subscription outright.
	if err := json.Unmarshal([]byte(out), &doc); err != nil {
		t.Fatalf("template output is not valid JSON: %v\n%s", err, out)
	}
	if len(doc.Outbounds) != 1 {
		t.Fatalf("got %d outbounds, want 1", len(doc.Outbounds))
	}
	if doc.Outbounds[0]["server"] != "de.example.com" {
		t.Errorf("outbound lost its server: %v", doc.Outbounds[0])
	}
	if len(doc.Selector) != 1 || doc.Selector[0] != "DE" {
		t.Errorf("tags not substituted: %v", doc.Selector)
	}
}

// A template that mentions no placeholder is served verbatim. That is a
// deliberate escape hatch, and the test pins it so nobody "fixes" it into a
// silent merge later.
func TestTemplateWithoutPlaceholdersIsVerbatim(t *testing.T) {
	const fixed = "mixed-port: 7890\nproxies: []\n"
	if got := RenderWith(templateBundle(), FormatClash, Templates{Clash: fixed}); got != fixed {
		t.Errorf("got %q, want it served as written", got)
	}
}
