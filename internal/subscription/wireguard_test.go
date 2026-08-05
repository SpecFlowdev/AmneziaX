package subscription

import (
	"strings"
	"testing"

	"github.com/SpecFlowdev/AmneziaX/internal/domain"
	"github.com/SpecFlowdev/AmneziaX/internal/xray"
)

func wgBundle(t *testing.T) Bundle {
	t.Helper()
	priv, _, err := xray.NewWireGuardKey()
	if err != nil {
		t.Fatal(err)
	}
	_, serverPub, err := xray.NewWireGuardKey()
	if err != nil {
		t.Fatal(err)
	}
	return Bundle{
		Title: "AmneziaX",
		User: &domain.User{
			Username:     "alice",
			WGPrivateKey: priv,
			WGIndex:      7,
		},
		Hosts: []domain.Host{{
			InboundType: "wireguard",
			Remark:      "NL",
			Address:     "nl.example.com",
			Port:        51820,
			PublicKey:   serverPub,
		}},
	}
}

func TestWireGuardConfHasEverySectionAClientNeeds(t *testing.T) {
	conf := WireGuardConf(wgBundle(t))

	// A WireGuard client refuses the file outright if either section is absent,
	// so their presence is the minimum bar, not a stylistic check.
	for _, want := range []string{
		"[Interface]", "PrivateKey = ", "Address = 10.66.", "DNS = ",
		"[Peer]", "PublicKey = ", "AllowedIPs = 0.0.0.0/0, ::/0",
		"Endpoint = nl.example.com:51820", "PersistentKeepalive = 25",
	} {
		if !strings.Contains(conf, want) {
			t.Errorf("missing %q in:\n%s", want, conf)
		}
	}

	// Interface must come before Peer: the file is parsed top-down and keys
	// before a section header belong to nothing.
	if strings.Index(conf, "[Interface]") > strings.Index(conf, "[Peer]") {
		t.Error("[Peer] appears before [Interface]")
	}
}

func TestWireGuardConfCarriesTheSubscribersOwnKey(t *testing.T) {
	b := wgBundle(t)
	conf := WireGuardConf(b)
	if !strings.Contains(conf, b.User.WGPrivateKey) {
		t.Fatal("the subscriber's private key is not in their own config")
	}
	if !strings.Contains(conf, b.Hosts[0].PublicKey) {
		t.Fatal("the server's public key is not in the peer section")
	}
	// The address has to match what the node was told to allow, or the tunnel
	// comes up and then drops every packet.
	if !strings.Contains(conf, xray.WireGuardAddress(b.User.WGIndex)) {
		t.Fatal("the address does not match the peer allowedIPs the node renders")
	}
}

func TestWireGuardConfSaysSoWhenThereIsNoServer(t *testing.T) {
	b := wgBundle(t)
	b.Hosts = nil
	if conf := WireGuardConf(b); !strings.HasPrefix(conf, "#") {
		t.Fatalf("expected a commented explanation, got:\n%s", conf)
	}

	// A disabled host must not be served either.
	b = wgBundle(t)
	b.Hosts[0].IsDisabled = true
	if conf := WireGuardConf(b); strings.Contains(conf, "[Peer]") {
		t.Fatal("a disabled host was served anyway")
	}
}

func TestWireGuardConfIgnoresNonWireGuardHosts(t *testing.T) {
	b := wgBundle(t)
	b.Hosts = append([]domain.Host{{
		InboundType: "vless", Remark: "not wg", Address: "x", Port: 443,
	}}, b.Hosts...)

	conf := WireGuardConf(b)
	if !strings.Contains(conf, "nl.example.com:51820") {
		t.Fatalf("picked the wrong host:\n%s", conf)
	}
	if strings.Count(conf, "[Peer]") != 1 {
		t.Fatal("more than one peer block: two peers claiming 0.0.0.0/0 behave differently per client")
	}
}
