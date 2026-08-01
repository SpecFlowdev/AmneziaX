package subscription

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/SpecFlowdev/AmneziaX/internal/domain"
)

// matrix covers every protocol and transport combination the panel can emit,
// because a malformed config is not rejected by the panel — it is rejected by
// the client, in a dialog the subscriber sees and the operator never does.
func matrix() []domain.Host {
	base := func(kind, name string) domain.Host {
		return domain.Host{
			InboundType: kind, InboundTag: kind + "-in", Remark: name,
			Address: "de.example.com", Port: 443,
		}
	}
	reality := func(h domain.Host) domain.Host {
		h.Security = "reality"
		h.SNI = "www.microsoft.com"
		h.PublicKey = "W5eMFnk7jEC_TX130qOipwz-pd66UcWqFSlYvZwMWjo"
		h.ShortID = "beef"
		h.Fingerprint = "chrome"
		return h
	}
	tls := func(h domain.Host) domain.Host {
		h.Security = "tls"
		h.SNI = "de.example.com"
		h.ALPN = "h2,http/1.1"
		h.Fingerprint = "firefox"
		return h
	}
	ws := func(h domain.Host) domain.Host {
		h.Path = "/ray"
		h.HostHeader = "cdn.example.com"
		return h
	}

	vlessVision := base("vless", "vless-reality-tcp")
	vlessVision.Flow = "xtls-rprx-vision"

	return []domain.Host{
		reality(vlessVision),
		tls(base("vless", "vless-tls-tcp")),
		ws(tls(base("vless", "vless-tls-ws"))),
		ws(base("vless", "vless-none-ws")),
		tls(base("trojan", "trojan-tls-tcp")),
		ws(tls(base("trojan", "trojan-tls-ws"))),
		tls(base("vmess", "vmess-tls-tcp")),
		ws(base("vmess", "vmess-none-ws")),
		base("shadowsocks", "ss-plain-tcp"),
	}
}

func TestXrayJSONShape(t *testing.T) {
	hosts := matrix()
	b := Bundle{User: testUser(), Hosts: hosts, Title: "AmneziaX"}

	var configs []map[string]any
	if err := json.Unmarshal([]byte(XrayJSON(b)), &configs); err != nil {
		t.Fatalf("output is not valid JSON: %v", err)
	}
	if len(configs) != len(hosts) {
		t.Fatalf("got %d configs for %d hosts", len(configs), len(hosts))
	}

	for i, c := range configs {
		// Clients list entries by remark; an unnamed one is unpickable.
		if remark, _ := c["remarks"].(string); remark == "" {
			t.Errorf("config %d has no remarks", i)
		}
		outbounds, _ := c["outbounds"].([]any)
		if len(outbounds) == 0 {
			t.Fatalf("config %d has no outbounds", i)
		}
		proxy, _ := outbounds[0].(map[string]any)
		if tag, _ := proxy["tag"].(string); tag != "proxy" {
			t.Errorf("config %d: first outbound is %q, want the proxy", i, tag)
		}
		if _, ok := proxy["streamSettings"]; !ok {
			t.Errorf("config %d: proxy has no streamSettings", i)
		}
	}
}

// A host the renderer cannot express must be dropped rather than emitted half
// built — a client reads a broken entry as a broken subscription.
func TestXrayJSONSkipsUnusableHosts(t *testing.T) {
	for _, h := range []domain.Host{
		{InboundType: "vless", Address: "", Port: 443},            // nowhere to connect
		{InboundType: "wireguard", Address: "a.example", Port: 1}, // not an xray protocol
	} {
		b := Bundle{User: testUser(), Hosts: []domain.Host{h}, Title: "AmneziaX"}
		var configs []map[string]any
		if err := json.Unmarshal([]byte(XrayJSON(b)), &configs); err != nil {
			t.Fatalf("output is not valid JSON: %v", err)
		}
		if len(configs) != 0 {
			t.Errorf("host %+v produced %d configs, want none", h, len(configs))
		}
	}
}

// TestXrayJSONAcceptedByXrayCore is the one that matters: xray-core itself
// parses what we emit. It is skipped unless XRAY_BIN points at a real binary,
// so the suite still runs on a machine without one.
//
//	XRAY_BIN=/usr/local/bin/xray go test ./internal/subscription/
func TestXrayJSONAcceptedByXrayCore(t *testing.T) {
	bin := os.Getenv("XRAY_BIN")
	if bin == "" {
		t.Skip("XRAY_BIN not set")
	}

	dir := t.TempDir()
	for _, h := range matrix() {
		t.Run(h.Remark, func(t *testing.T) {
			b := Bundle{User: testUser(), Hosts: []domain.Host{h}, Title: "AmneziaX"}

			var configs []json.RawMessage
			if err := json.Unmarshal([]byte(XrayJSON(b)), &configs); err != nil {
				t.Fatalf("output is not valid JSON: %v", err)
			}
			if len(configs) != 1 {
				t.Fatalf("got %d configs, want 1", len(configs))
			}

			path := filepath.Join(dir, h.Remark+".json")
			if err := os.WriteFile(path, configs[0], 0o600); err != nil {
				t.Fatal(err)
			}
			out, err := exec.Command(bin, "run", "-test", "-c", path).CombinedOutput()
			if err != nil {
				t.Fatalf("xray rejected the config: %v\n%s", err, out)
			}
		})
	}
}

// DetectClientFormat has to say "I don't know" rather than guessing, because
// the caller's fallback is an operator-chosen format that a Clash app cannot
// read. Collapsing the two would hand that default to clients that need their
// own.
func TestDetectClientFormat(t *testing.T) {
	for _, tc := range []struct {
		ua   string
		want Format
		ok   bool
	}{
		{ua: "clash-verge/1.5", want: FormatClash, ok: true},
		{ua: "mihomo/1.18", want: FormatClash, ok: true},
		{ua: "Stash/2.6", want: FormatClash, ok: true},
		{ua: "sing-box/1.9", want: FormatSingBox, ok: true},
		{ua: "Hiddify/2.0", want: FormatSingBox, ok: true},
		{ua: "karing/1.0", want: FormatSingBox, ok: true},

		// Not a claim about what these want — a claim that we do not know, so
		// the panel's configured default applies.
		{ua: "v2rayNG/1.8.5", ok: false},
		{ua: "Streisand/1.6", ok: false},
		{ua: "", ok: false},
	} {
		t.Run(tc.ua, func(t *testing.T) {
			got, ok := DetectClientFormat(tc.ua)
			if ok != tc.ok {
				t.Fatalf("DetectClientFormat(%q) recognised=%v, want %v", tc.ua, ok, tc.ok)
			}
			if ok && got != tc.want {
				t.Fatalf("DetectClientFormat(%q) = %q, want %q", tc.ua, got, tc.want)
			}
		})
	}
}
