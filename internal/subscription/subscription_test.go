package subscription

import (
	"encoding/base64"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/SpecFlowdev/AmneziaX/internal/domain"
)

func testUser() *domain.User {
	expire := time.Now().Add(48 * time.Hour)
	return &domain.User{
		UUID:              "8d2d1a2c-4d5f-4e6a-9b8c-1d2e3f4a5b6c",
		Username:          "alice",
		Tag:               "vip",
		VlessUUID:         "11111111-2222-3333-4444-555555555555",
		TrojanPassword:    "trojan-secret",
		SSPassword:        "ss-secret",
		Status:            domain.UserActive,
		UsedTrafficBytes:  1024,
		TrafficLimitBytes: 4096,
		ExpireAt:          &expire,
	}
}

func realityHost() domain.Host {
	return domain.Host{
		InboundTag:  "vless-reality",
		InboundType: "vless",
		ProfileName: "Default",
		Remark:      "NL-{{USERNAME}}",
		Address:     "nl.example.com",
		Port:        443,
		Security:    "reality",
		SNI:         "www.google.com",
		Fingerprint: "chrome",
		PublicKey:   "pubkey",
		ShortID:     "ab12",
		Flow:        "xtls-rprx-vision",
	}
}

func TestVlessLinkCarriesRealityParameters(t *testing.T) {
	links := Links(Bundle{User: testUser(), Hosts: []domain.Host{realityHost()}})
	if len(links) != 1 {
		t.Fatalf("got %d links, want 1", len(links))
	}

	link := links[0]
	if !strings.HasPrefix(link, "vless://11111111-2222-3333-4444-555555555555@nl.example.com:443?") {
		t.Fatalf("unexpected link prefix: %s", link)
	}

	parsed, err := url.Parse(link)
	if err != nil {
		t.Fatalf("the link is not a valid URI: %v", err)
	}
	q := parsed.Query()
	for key, want := range map[string]string{
		"security": "reality",
		"sni":      "www.google.com",
		"fp":       "chrome",
		"pbk":      "pubkey",
		"sid":      "ab12",
		"flow":     "xtls-rprx-vision",
		"type":     "tcp",
	} {
		if got := q.Get(key); got != want {
			t.Errorf("query %s = %q, want %q", key, got, want)
		}
	}
}

func TestRemarkPlaceholdersAreExpanded(t *testing.T) {
	links := Links(Bundle{User: testUser(), Hosts: []domain.Host{realityHost()}})
	if !strings.HasSuffix(links[0], "#NL-alice") {
		t.Fatalf("the {{USERNAME}} placeholder was not expanded: %s", links[0])
	}
}

func TestWebsocketHostImpliesWsTransport(t *testing.T) {
	host := realityHost()
	host.Path = "/ws"
	host.Security = "tls"

	parsed, err := url.Parse(Links(Bundle{User: testUser(), Hosts: []domain.Host{host}})[0])
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if got := parsed.Query().Get("type"); got != "ws" {
		t.Fatalf("type = %q, want ws", got)
	}
	if got := parsed.Query().Get("path"); got != "/ws" {
		t.Fatalf("path = %q, want /ws", got)
	}
}

func TestTrojanAndShadowsocksLinks(t *testing.T) {
	trojan := realityHost()
	trojan.InboundType = "trojan"
	ss := realityHost()
	ss.InboundType = "shadowsocks"

	links := Links(Bundle{User: testUser(), Hosts: []domain.Host{trojan, ss}})
	if len(links) != 2 {
		t.Fatalf("got %d links, want 2", len(links))
	}
	if !strings.HasPrefix(links[0], "trojan://trojan-secret@") {
		t.Fatalf("unexpected trojan link: %s", links[0])
	}
	if !strings.HasPrefix(links[1], "ss://") {
		t.Fatalf("unexpected shadowsocks link: %s", links[1])
	}
	userinfo := strings.TrimPrefix(strings.SplitN(links[1], "@", 2)[0], "ss://")
	decoded, err := base64.RawURLEncoding.DecodeString(userinfo)
	if err != nil {
		t.Fatalf("the shadowsocks userinfo is not base64url: %v", err)
	}
	if !strings.HasSuffix(string(decoded), ":ss-secret") {
		t.Fatalf("unexpected shadowsocks userinfo: %s", decoded)
	}
}

func TestUnknownProtocolsAreSkipped(t *testing.T) {
	host := realityHost()
	host.InboundType = "socks"
	if links := Links(Bundle{User: testUser(), Hosts: []domain.Host{host}}); len(links) != 0 {
		t.Fatalf("expected no links for an unsupported protocol, got %v", links)
	}
}

func TestHostWithoutAddressIsSkipped(t *testing.T) {
	host := realityHost()
	host.Address = ""
	if links := Links(Bundle{User: testUser(), Hosts: []domain.Host{host}}); len(links) != 0 {
		t.Fatalf("expected no links for an address-less host, got %v", links)
	}
}

func TestBase64PayloadDecodesToTheLinkList(t *testing.T) {
	bundle := Bundle{User: testUser(), Hosts: []domain.Host{realityHost()}}
	decoded, err := base64.StdEncoding.DecodeString(Base64(bundle))
	if err != nil {
		t.Fatalf("the subscription payload is not base64: %v", err)
	}
	if string(decoded) != strings.Join(Links(bundle), "\n") {
		t.Fatal("the base64 payload must match the plain link list")
	}
}

func TestHeadersReportQuotaAndExpiry(t *testing.T) {
	user := testUser()
	headers := Headers(Bundle{User: user, Title: "AmneziaX"})

	info := headers["subscription-userinfo"]
	if !strings.Contains(info, "download=1024") || !strings.Contains(info, "total=4096") {
		t.Fatalf("unexpected subscription-userinfo: %s", info)
	}
	if !strings.Contains(info, "expire="+strconv.FormatInt(user.ExpireAt.Unix(), 10)) {
		t.Fatalf("the expiry is missing from subscription-userinfo: %s", info)
	}
	if _, ok := headers["profile-web-page-url"]; ok {
		t.Fatal("an empty support URL must not produce a header")
	}
	title := strings.TrimPrefix(headers["profile-title"], "base64:")
	decoded, err := base64.StdEncoding.DecodeString(title)
	if err != nil || string(decoded) != "AmneziaX" {
		t.Fatalf("unexpected profile-title: %s", headers["profile-title"])
	}
}

func TestBuildInfoComputesDaysLeft(t *testing.T) {
	info := BuildInfo(Bundle{User: testUser(), Hosts: []domain.Host{realityHost()}}, "https://p/sub/x")
	if info.DaysLeft == nil || *info.DaysLeft != 1 {
		t.Fatalf("daysLeft = %v, want 1 (48h rounds down to one full day)", info.DaysLeft)
	}
	if len(info.Links) != 1 {
		t.Fatalf("got %d links, want 1", len(info.Links))
	}
}
