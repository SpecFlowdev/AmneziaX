package subscription

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/SpecFlowdev/AmneziaX/internal/domain"
)

// Format is a subscription encoding a client understands.
type Format string

const (
	FormatBase64  Format = "base64"
	FormatPlain   Format = "plain"
	FormatClash   Format = "clash"
	FormatSingBox Format = "singbox"
	FormatJSON    Format = "json"
)

func (f Format) Valid() bool {
	switch f {
	case FormatBase64, FormatPlain, FormatClash, FormatSingBox, FormatJSON:
		return true
	}
	return false
}

// ContentType is what the response should be served as.
func (f Format) ContentType() string {
	switch f {
	case FormatClash:
		return "text/yaml; charset=utf-8"
	case FormatSingBox, FormatJSON:
		return "application/json; charset=utf-8"
	default:
		return "text/plain; charset=utf-8"
	}
}

// DetectClientFormat reports the format a client needs when it names itself.
// The second return distinguishes "this client requires Clash" from "no idea
// what this is" — the caller has a configured default for the second case, and
// collapsing the two would apply that default to clients that cannot read it.
func DetectClientFormat(userAgent string) (Format, bool) {
	ua := strings.ToLower(userAgent)
	switch {
	case strings.Contains(ua, "clash") || strings.Contains(ua, "mihomo") ||
		strings.Contains(ua, "stash") || strings.Contains(ua, "flclash"):
		return FormatClash, true
	case strings.Contains(ua, "sing-box") || strings.Contains(ua, "singbox") ||
		strings.Contains(ua, "hiddify") || strings.Contains(ua, "karing"):
		return FormatSingBox, true
	default:
		return "", false
	}
}

// DetectFormat picks an encoding from the client's User-Agent, falling back to
// the base64 list that every client understands.
func DetectFormat(userAgent string) Format {
	if f, ok := DetectClientFormat(userAgent); ok {
		return f
	}
	return FormatBase64
}

// ---------------------------------------------------------------- clash

// Clash renders a Mihomo/Clash.Meta profile. It is emitted as YAML by hand
// because the document is small, fixed in shape, and hand-writing it avoids a
// YAML dependency in the panel.
func Clash(b Bundle) string {
	var sb strings.Builder
	sb.WriteString("# " + yamlComment(b.Title) + "\n")
	sb.WriteString("mixed-port: 7890\nallow-lan: false\nmode: rule\nlog-level: warning\n")
	sb.WriteString("external-controller: 127.0.0.1:9090\n\n")

	names := make([]string, 0, len(b.Hosts))
	sb.WriteString("proxies:\n")
	for _, h := range b.Hosts {
		entry, name := clashProxy(b.User, h)
		if entry == "" {
			continue
		}
		names = append(names, name)
		sb.WriteString(entry)
	}
	if len(names) == 0 {
		// A profile with no proxies is invalid; DIRECT keeps the client working
		// instead of showing a parse error.
		sb.WriteString("  - {name: DIRECT-ONLY, type: direct}\n")
		names = append(names, "DIRECT-ONLY")
	}

	quoted := make([]string, 0, len(names))
	for _, n := range names {
		quoted = append(quoted, yamlString(n))
	}
	list := strings.Join(quoted, ", ")

	fmt.Fprintf(&sb, "\nproxy-groups:\n")
	fmt.Fprintf(&sb, "  - {name: %s, type: select, proxies: [%s]}\n", yamlString(b.Title), list)
	fmt.Fprintf(&sb, "  - {name: AUTO, type: url-test, url: 'http://www.gstatic.com/generate_204', interval: 300, proxies: [%s]}\n", list)

	sb.WriteString("\nrules:\n")
	sb.WriteString("  - GEOIP,private,DIRECT,no-resolve\n")
	fmt.Fprintf(&sb, "  - MATCH,%s\n", yamlString(b.Title))
	return sb.String()
}

func clashProxy(u *domain.User, h domain.Host) (entry, name string) {
	if h.Address == "" {
		return "", ""
	}
	name = expandRemark(orDefault(h.Remark, h.InboundTag), u, h)
	network := networkOf(h)

	var sb strings.Builder
	switch strings.ToLower(h.InboundType) {
	case "vless":
		fmt.Fprintf(&sb, "  - {name: %s, type: vless, server: %s, port: %d, uuid: %s, udp: true, tls: %t",
			yamlString(name), h.Address, portOr443(h.Port), u.VlessUUID, h.Security != "" && h.Security != "none")
		if h.Flow != "" {
			fmt.Fprintf(&sb, ", flow: %s", h.Flow)
		}
		if h.Security == "reality" {
			fmt.Fprintf(&sb, ", servername: %s, client-fingerprint: %s, reality-opts: {public-key: %s",
				h.SNI, orDefault(h.Fingerprint, "chrome"), h.PublicKey)
			if h.ShortID != "" {
				fmt.Fprintf(&sb, ", short-id: %s", h.ShortID)
			}
			sb.WriteString("}")
		} else if h.SNI != "" {
			fmt.Fprintf(&sb, ", servername: %s", h.SNI)
		}
	case "trojan":
		fmt.Fprintf(&sb, "  - {name: %s, type: trojan, server: %s, port: %d, password: %s, udp: true",
			yamlString(name), h.Address, portOr443(h.Port), yamlString(u.TrojanPassword))
		if h.SNI != "" {
			fmt.Fprintf(&sb, ", sni: %s", h.SNI)
		}
	case "shadowsocks":
		fmt.Fprintf(&sb, "  - {name: %s, type: ss, server: %s, port: %d, cipher: chacha20-ietf-poly1305, password: %s, udp: true",
			yamlString(name), h.Address, portOr443(h.Port), yamlString(u.SSPassword))
	case "vmess":
		fmt.Fprintf(&sb, "  - {name: %s, type: vmess, server: %s, port: %d, uuid: %s, alterId: 0, cipher: auto, udp: true",
			yamlString(name), h.Address, portOr443(h.Port), u.VlessUUID)
		if h.SNI != "" {
			fmt.Fprintf(&sb, ", servername: %s", h.SNI)
		}
	default:
		return "", ""
	}

	if h.AllowInsecure {
		sb.WriteString(", skip-cert-verify: true")
	}
	if network == "ws" {
		fmt.Fprintf(&sb, ", network: ws, ws-opts: {path: %s", yamlString(h.Path))
		if h.HostHeader != "" {
			fmt.Fprintf(&sb, ", headers: {Host: %s}", h.HostHeader)
		}
		sb.WriteString("}")
	}
	sb.WriteString("}\n")
	return sb.String(), name
}

// ---------------------------------------------------------------- sing-box

// SingBox renders a sing-box client configuration with a selector and a
// urltest group over every host.
func SingBox(b Bundle) string {
	outbounds := []map[string]any{}
	tags := []string{}

	for _, h := range b.Hosts {
		ob, tag := singBoxOutbound(b.User, h)
		if ob == nil {
			continue
		}
		outbounds = append(outbounds, ob)
		tags = append(tags, tag)
	}

	selector := map[string]any{
		"type": "selector", "tag": b.Title,
		"outbounds": append(append([]string{"auto"}, tags...), "direct"),
		"default":   "auto",
	}
	auto := map[string]any{
		"type": "urltest", "tag": "auto", "outbounds": tags,
		"url": "http://www.gstatic.com/generate_204", "interval": "5m",
	}
	if len(tags) == 0 {
		selector["outbounds"] = []string{"direct"}
		selector["default"] = "direct"
		auto = nil
	}

	all := []map[string]any{selector}
	if auto != nil {
		all = append(all, auto)
	}
	all = append(all, outbounds...)
	all = append(all,
		map[string]any{"type": "direct", "tag": "direct"},
		map[string]any{"type": "block", "tag": "block"},
	)

	doc := map[string]any{
		"log": map[string]any{"level": "warn", "timestamp": true},
		"dns": map[string]any{
			"servers": []any{
				map[string]any{"tag": "remote", "address": "https://1.1.1.1/dns-query", "detour": b.Title},
				map[string]any{"tag": "local", "address": "local", "detour": "direct"},
			},
			"rules":    []any{map[string]any{"outbound": "any", "server": "local"}},
			"strategy": "prefer_ipv4",
		},
		"inbounds": []any{
			map[string]any{
				"type": "tun", "tag": "tun-in", "address": []string{"172.19.0.1/30"},
				"auto_route": true, "strict_route": true, "stack": "mixed",
			},
			map[string]any{"type": "mixed", "tag": "mixed-in", "listen": "127.0.0.1", "listen_port": 2080},
		},
		"outbounds": all,
		"route": map[string]any{
			"auto_detect_interface": true,
			"rules": []any{
				map[string]any{"action": "sniff"},
				map[string]any{"protocol": "dns", "action": "hijack-dns"},
				map[string]any{"ip_is_private": true, "outbound": "direct"},
			},
		},
	}

	encoded, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return "{}"
	}
	return string(encoded)
}

func singBoxOutbound(u *domain.User, h domain.Host) (map[string]any, string) {
	if h.Address == "" {
		return nil, ""
	}
	tag := expandRemark(orDefault(h.Remark, h.InboundTag), u, h)
	ob := map[string]any{
		"tag":         tag,
		"server":      h.Address,
		"server_port": portOr443(h.Port),
	}

	switch strings.ToLower(h.InboundType) {
	case "vless":
		ob["type"] = "vless"
		ob["uuid"] = u.VlessUUID
		if h.Flow != "" {
			ob["flow"] = h.Flow
		}
	case "trojan":
		ob["type"] = "trojan"
		ob["password"] = u.TrojanPassword
	case "shadowsocks":
		ob["type"] = "shadowsocks"
		ob["method"] = "chacha20-ietf-poly1305"
		ob["password"] = u.SSPassword
	case "vmess":
		ob["type"] = "vmess"
		ob["uuid"] = u.VlessUUID
		ob["alter_id"] = 0
		ob["security"] = "auto"
	default:
		return nil, ""
	}

	if h.Security != "" && h.Security != "none" {
		tls := map[string]any{"enabled": true}
		if h.SNI != "" {
			tls["server_name"] = h.SNI
		}
		if h.AllowInsecure {
			tls["insecure"] = true
		}
		if h.ALPN != "" {
			tls["alpn"] = strings.Split(h.ALPN, ",")
		}
		if h.Security == "reality" {
			reality := map[string]any{"enabled": true, "public_key": h.PublicKey}
			if h.ShortID != "" {
				reality["short_id"] = h.ShortID
			}
			tls["reality"] = reality
			tls["utls"] = map[string]any{"enabled": true, "fingerprint": orDefault(h.Fingerprint, "chrome")}
		} else if h.Fingerprint != "" {
			tls["utls"] = map[string]any{"enabled": true, "fingerprint": h.Fingerprint}
		}
		ob["tls"] = tls
	}

	if networkOf(h) == "ws" {
		transport := map[string]any{"type": "ws", "path": h.Path}
		if h.HostHeader != "" {
			transport["headers"] = map[string]any{"Host": h.HostHeader}
		}
		ob["transport"] = transport
	}
	return ob, tag
}

// ---------------------------------------------------------------- xray json

// XrayJSON renders the "JSON subscription" that v2rayN, v2rayNG, Happ and
// Streisand accept: an array of complete Xray client configurations, one per
// host, each ready to run as-is. It is the most faithful of the formats — a
// vless:// link has to squeeze every stream setting through a query string,
// while this carries the same structure xray-core actually parses.
//
// Every entry is self-contained on purpose. Clients present them as a server
// list and switch between them, so an entry that leaned on a shared block would
// stop working the moment the user picked a different one.
func XrayJSON(b Bundle) string {
	configs := make([]map[string]any, 0, len(b.Hosts))
	for _, h := range b.Hosts {
		proxy, remark := xrayOutbound(b.User, h)
		if proxy == nil {
			continue
		}
		configs = append(configs, map[string]any{
			"remarks": remark,
			"log":     map[string]any{"loglevel": "warning"},
			"inbounds": []any{
				map[string]any{
					"tag": "socks", "protocol": "socks",
					"listen": "127.0.0.1", "port": 10808,
					"settings": map[string]any{"udp": true, "auth": "noauth"},
					"sniffing": map[string]any{
						"enabled":      true,
						"destOverride": []string{"http", "tls", "quic"},
						"routeOnly":    false,
					},
				},
				map[string]any{
					"tag": "http", "protocol": "http",
					"listen": "127.0.0.1", "port": 10809,
					"settings": map[string]any{},
				},
			},
			"outbounds": []any{
				proxy,
				map[string]any{"tag": "direct", "protocol": "freedom", "settings": map[string]any{}},
				map[string]any{"tag": "block", "protocol": "blackhole", "settings": map[string]any{
					"response": map[string]any{"type": "http"},
				}},
			},
			"dns": map[string]any{"servers": []any{"1.1.1.1", "8.8.8.8", "localhost"}},
			"routing": map[string]any{
				"domainStrategy": "IPIfNonMatch",
				"rules": []any{
					// Torrents over someone else's exit node get the node
					// blocked, so they never leave through the proxy.
					map[string]any{"type": "field", "outboundTag": "block", "protocol": []string{"bittorrent"}},
					map[string]any{"type": "field", "outboundTag": "direct", "ip": []string{"geoip:private"}},
				},
			},
		})
	}

	// An empty array parses but leaves the client with nothing and no
	// explanation. `null` is worse. Either way the caller sees valid JSON.
	encoded, err := json.MarshalIndent(configs, "", "  ")
	if err != nil {
		return "[]"
	}
	return string(encoded)
}

// xrayOutbound maps one host onto the proxy outbound of an Xray config.
func xrayOutbound(u *domain.User, h domain.Host) (map[string]any, string) {
	if h.Address == "" {
		return nil, ""
	}
	remark := expandRemark(orDefault(h.Remark, h.InboundTag), u, h)
	port := portOr443(h.Port)

	ob := map[string]any{"tag": "proxy"}
	switch strings.ToLower(h.InboundType) {
	case "vless":
		user := map[string]any{"id": u.VlessUUID, "encryption": "none", "level": 0}
		if h.Flow != "" {
			user["flow"] = h.Flow
		}
		ob["protocol"] = "vless"
		ob["settings"] = map[string]any{"vnext": []any{map[string]any{
			"address": h.Address, "port": port, "users": []any{user},
		}}}
	case "vmess":
		ob["protocol"] = "vmess"
		ob["settings"] = map[string]any{"vnext": []any{map[string]any{
			"address": h.Address, "port": port,
			"users": []any{map[string]any{
				"id": u.VlessUUID, "alterId": 0, "security": "auto", "level": 0,
			}},
		}}}
	case "trojan":
		ob["protocol"] = "trojan"
		ob["settings"] = map[string]any{"servers": []any{map[string]any{
			"address": h.Address, "port": port, "password": u.TrojanPassword, "level": 0,
		}}}
	case "shadowsocks":
		ob["protocol"] = "shadowsocks"
		ob["settings"] = map[string]any{"servers": []any{map[string]any{
			"address": h.Address, "port": port,
			"method": "chacha20-ietf-poly1305", "password": u.SSPassword, "level": 0,
		}}}
	default:
		return nil, ""
	}

	ob["streamSettings"] = xrayStreamSettings(h)
	ob["mux"] = map[string]any{"enabled": false, "concurrency": -1}
	return ob, remark
}

func xrayStreamSettings(h domain.Host) map[string]any {
	network := networkOf(h)
	stream := map[string]any{"network": network}

	switch {
	case h.Security == "reality":
		stream["security"] = "reality"
		reality := map[string]any{
			"publicKey":   h.PublicKey,
			"fingerprint": orDefault(h.Fingerprint, "chrome"),
			"show":        false,
		}
		if h.SNI != "" {
			reality["serverName"] = h.SNI
		}
		if h.ShortID != "" {
			reality["shortId"] = h.ShortID
		}
		if h.SpiderX != "" {
			reality["spiderX"] = h.SpiderX
		}
		stream["realitySettings"] = reality
	case h.Security != "" && h.Security != "none":
		stream["security"] = h.Security
		tls := map[string]any{"allowInsecure": h.AllowInsecure}
		if h.SNI != "" {
			tls["serverName"] = h.SNI
		}
		if h.Fingerprint != "" {
			tls["fingerprint"] = h.Fingerprint
		}
		if h.ALPN != "" {
			tls["alpn"] = strings.Split(h.ALPN, ",")
		}
		stream["tlsSettings"] = tls
	default:
		stream["security"] = "none"
	}

	if network == "ws" {
		ws := map[string]any{"path": h.Path}
		if h.HostHeader != "" {
			ws["headers"] = map[string]any{"Host": h.HostHeader}
		}
		stream["wsSettings"] = ws
	}
	return stream
}

// ---------------------------------------------------------------- helpers

func orDefault(v, fallback string) string {
	if strings.TrimSpace(v) == "" {
		return fallback
	}
	return v
}

func portOr443(p int) int {
	if p <= 0 {
		return 443
	}
	return p
}

// yamlString quotes a scalar so names containing colons, quotes or leading
// symbols cannot break the document.
func yamlString(v string) string {
	return strconv.Quote(v)
}

func yamlComment(v string) string {
	return strings.ReplaceAll(strings.ReplaceAll(v, "\n", " "), "\r", "")
}

// Render produces the payload for a format.
func Render(b Bundle, f Format) string {
	switch f {
	case FormatClash:
		return Clash(b)
	case FormatSingBox:
		return SingBox(b)
	case FormatJSON:
		return XrayJSON(b)
	case FormatPlain:
		return strings.Join(Links(b), "\n")
	default:
		return Base64(b)
	}
}
