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

// DetectFormat picks an encoding from the client's User-Agent. Clients that
// announce themselves get their native format; everything else falls back to
// the base64 list, which is universally understood.
func DetectFormat(userAgent string) Format {
	ua := strings.ToLower(userAgent)
	switch {
	case strings.Contains(ua, "clash") || strings.Contains(ua, "mihomo") ||
		strings.Contains(ua, "stash") || strings.Contains(ua, "flclash"):
		return FormatClash
	case strings.Contains(ua, "sing-box") || strings.Contains(ua, "singbox") ||
		strings.Contains(ua, "hiddify") || strings.Contains(ua, "karing"):
		return FormatSingBox
	default:
		return FormatBase64
	}
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
	case FormatPlain:
		return strings.Join(Links(b), "\n")
	default:
		return Base64(b)
	}
}
