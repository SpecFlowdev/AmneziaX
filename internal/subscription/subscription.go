// Package subscription turns a user's hosts into client-ready connection links.
package subscription

import (
	"encoding/base64"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/SpecFlowdev/AmneziaX/internal/domain"
)

// Bundle is everything a subscription response is built from.
type Bundle struct {
	User  *domain.User
	Hosts []domain.Host
	// Title is shown as the profile name in client apps.
	Title      string
	SupportURL string
	// UpdateInterval is how often clients should refresh, in hours.
	UpdateInterval int
}

// Links renders one connection URI per enabled host.
func Links(b Bundle) []string {
	out := make([]string, 0, len(b.Hosts))
	for _, h := range b.Hosts {
		if link := hostLink(b.User, h); link != "" {
			out = append(out, link)
		}
	}
	return out
}

// Base64 renders the classic base64 subscription payload understood by nearly
// every client.
func Base64(b Bundle) string {
	return base64.StdEncoding.EncodeToString([]byte(strings.Join(Links(b), "\n")))
}

func hostLink(u *domain.User, h domain.Host) string {
	address := h.Address
	if address == "" {
		return ""
	}
	remark := h.Remark
	if remark == "" {
		remark = h.InboundTag
	}
	remark = expandRemark(remark, u, h)

	switch strings.ToLower(h.InboundType) {
	case "vless":
		return vlessLink(u, h, address, remark)
	case "trojan":
		return trojanLink(u, h, address, remark)
	case "shadowsocks":
		return shadowsocksLink(u, h, address, remark)
	case "vmess":
		// vmess:// carries a base64 JSON blob rather than a query string.
		return vmessLink(u, h, address, remark)
	default:
		return ""
	}
}

// expandRemark substitutes the placeholders an operator can use in a host
// remark so one host row can produce per-user labels.
func expandRemark(remark string, u *domain.User, h domain.Host) string {
	repl := strings.NewReplacer(
		"{{USERNAME}}", u.Username,
		"{{TAG}}", u.Tag,
		"{{INBOUND}}", h.InboundTag,
		"{{PROFILE}}", h.ProfileName,
	)
	return repl.Replace(remark)
}

func streamQuery(h domain.Host, network string) url.Values {
	q := url.Values{}
	if network == "" {
		network = "tcp"
	}
	q.Set("type", network)

	security := h.Security
	if security == "" {
		security = "none"
	}
	q.Set("security", security)

	if h.SNI != "" {
		q.Set("sni", h.SNI)
	}
	if h.HostHeader != "" {
		q.Set("host", h.HostHeader)
	}
	if h.Path != "" {
		q.Set("path", h.Path)
	}
	if h.ALPN != "" {
		q.Set("alpn", h.ALPN)
	}
	if h.Fingerprint != "" {
		q.Set("fp", h.Fingerprint)
	}
	if h.PublicKey != "" {
		q.Set("pbk", h.PublicKey)
	}
	if h.ShortID != "" {
		q.Set("sid", h.ShortID)
	}
	if h.SpiderX != "" {
		q.Set("spx", h.SpiderX)
	}
	if h.AllowInsecure {
		q.Set("allowInsecure", "1")
	}
	return q
}

// networkOf derives the transport from the host's own hints. Hosts store the
// client-visible view, so a path implies a websocket-like transport.
func networkOf(h domain.Host) string {
	switch {
	case h.Path != "" && strings.HasPrefix(strings.ToLower(h.Path), "/"):
		return "ws"
	default:
		return "tcp"
	}
}

func vlessLink(u *domain.User, h domain.Host, address, remark string) string {
	q := streamQuery(h, networkOf(h))
	if h.Flow != "" {
		q.Set("flow", h.Flow)
	}
	return fmt.Sprintf("vless://%s@%s?%s#%s",
		u.VlessUUID, hostPort(address, h.Port), q.Encode(), url.PathEscape(remark))
}

func trojanLink(u *domain.User, h domain.Host, address, remark string) string {
	q := streamQuery(h, networkOf(h))
	return fmt.Sprintf("trojan://%s@%s?%s#%s",
		url.QueryEscape(u.TrojanPassword), hostPort(address, h.Port), q.Encode(), url.PathEscape(remark))
}

func shadowsocksLink(u *domain.User, h domain.Host, address, remark string) string {
	// The userinfo section is method:password, base64 encoded.
	userinfo := base64.RawURLEncoding.EncodeToString([]byte("chacha20-ietf-poly1305:" + u.SSPassword))
	return fmt.Sprintf("ss://%s@%s#%s", userinfo, hostPort(address, h.Port), url.PathEscape(remark))
}

func vmessLink(u *domain.User, h domain.Host, address, remark string) string {
	fields := map[string]string{
		"v":    "2",
		"ps":   remark,
		"add":  address,
		"port": strconv.Itoa(h.Port),
		"id":   u.VlessUUID,
		"aid":  "0",
		"scy":  "auto",
		"net":  networkOf(h),
		"type": "none",
		"host": h.HostHeader,
		"path": h.Path,
		"tls":  h.Security,
		"sni":  h.SNI,
		"alpn": h.ALPN,
		"fp":   h.Fingerprint,
	}
	parts := make([]string, 0, len(fields))
	for _, k := range []string{"v", "ps", "add", "port", "id", "aid", "scy", "net", "type", "host", "path", "tls", "sni", "alpn", "fp"} {
		parts = append(parts, fmt.Sprintf("%q:%q", k, fields[k]))
	}
	blob := "{" + strings.Join(parts, ",") + "}"
	return "vmess://" + base64.StdEncoding.EncodeToString([]byte(blob))
}

func hostPort(address string, port int) string {
	if port <= 0 {
		port = 443
	}
	return net.JoinHostPort(address, strconv.Itoa(port))
}

// Headers are the subscription-userinfo style metadata clients read to display
// quota and expiry without opening the panel.
func Headers(b Bundle) map[string]string {
	u := b.User
	var expire int64
	if u.ExpireAt != nil {
		expire = u.ExpireAt.Unix()
	}
	interval := b.UpdateInterval
	if interval <= 0 {
		interval = 12
	}
	h := map[string]string{
		"profile-title":           base64Title(b.Title),
		"profile-update-interval": strconv.Itoa(interval),
		"subscription-userinfo":   fmt.Sprintf("upload=0; download=%d; total=%d; expire=%d", u.UsedTrafficBytes, u.TrafficLimitBytes, expire),
		"profile-web-page-url":    b.SupportURL,
		"content-disposition":     fmt.Sprintf("attachment; filename=%q", u.Username),
	}
	if b.SupportURL == "" {
		delete(h, "profile-web-page-url")
	}
	return h
}

func base64Title(title string) string {
	if title == "" {
		title = "AmneziaX"
	}
	return "base64:" + base64.StdEncoding.EncodeToString([]byte(title))
}

// Info is the JSON payload the subscription page renders.
type Info struct {
	Username        string     `json:"username"`
	Status          string     `json:"status"`
	UsedBytes       int64      `json:"usedTrafficBytes"`
	LimitBytes      int64      `json:"trafficLimitBytes"`
	LifetimeBytes   int64      `json:"lifetimeUsedTrafficBytes"`
	ExpireAt        *time.Time `json:"expireAt"`
	SubscriptionURL string     `json:"subscriptionUrl"`
	Links           []string   `json:"links"`
	Title           string     `json:"title"`
	SupportURL      string     `json:"supportUrl,omitempty"`
	DaysLeft        *int       `json:"daysLeft,omitempty"`
	// Notices an operator has published. Empty on a deployment that has never
	// used them, so the page simply renders nothing.
	Announcements []domain.Announcement `json:"announcements,omitempty"`
	// Page options, so the subscriber page renders what the operator chose
	// without a second request.
	ShowLinks   bool `json:"showLinks"`
	ShowFormats bool `json:"showFormats"`
}

func BuildInfo(b Bundle, subURL string) Info {
	i := Info{
		Username:        b.User.Username,
		Status:          string(b.User.Status),
		UsedBytes:       b.User.UsedTrafficBytes,
		LimitBytes:      b.User.TrafficLimitBytes,
		LifetimeBytes:   b.User.LifetimeTrafficBytes,
		ExpireAt:        b.User.ExpireAt,
		SubscriptionURL: subURL,
		Links:           Links(b),
		Title:           b.Title,
		SupportURL:      b.SupportURL,
	}
	if b.User.ExpireAt != nil {
		days := int(time.Until(*b.User.ExpireAt).Hours() / 24)
		i.DaysLeft = &days
	}
	return i
}
