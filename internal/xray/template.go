package xray

import (
	"encoding/json"
	"fmt"
)

// TemplateOptions parameterises the starter profile created on first boot.
type TemplateOptions struct {
	InboundTag  string
	Port        int
	ServerNames []string
	PrivateKey  string
	ShortIDs    []string
	Dest        string
}

// DefaultTemplate builds a VLESS + REALITY profile that is ready to serve
// traffic as soon as a node picks it up, so a fresh install is never staring at
// an empty editor.
func DefaultTemplate(opts TemplateOptions) (json.RawMessage, error) {
	if opts.InboundTag == "" {
		opts.InboundTag = "vless-reality"
	}
	if opts.Port == 0 {
		opts.Port = 443
	}
	if len(opts.ServerNames) == 0 {
		opts.ServerNames = []string{"www.google.com"}
	}
	if opts.Dest == "" {
		opts.Dest = opts.ServerNames[0] + ":443"
	}
	if opts.PrivateKey == "" {
		pair, err := GenerateRealityKeys()
		if err != nil {
			return nil, err
		}
		opts.PrivateKey = pair.PrivateKey
	}
	if len(opts.ShortIDs) == 0 {
		ids, err := GenerateShortIDs(8)
		if err != nil {
			return nil, err
		}
		opts.ShortIDs = ids
	}

	doc := map[string]any{
		"log": map[string]any{"loglevel": "warning"},
		"dns": map[string]any{
			"servers": []any{"https://1.1.1.1/dns-query", "1.1.1.1", "8.8.8.8", "localhost"},
		},
		"inbounds": []any{
			map[string]any{
				"tag":      opts.InboundTag,
				"listen":   "0.0.0.0",
				"port":     opts.Port,
				"protocol": "vless",
				"settings": map[string]any{
					"clients":    []any{},
					"decryption": "none",
				},
				"streamSettings": map[string]any{
					"network":  "tcp",
					"security": "reality",
					"realitySettings": map[string]any{
						"show":        false,
						"dest":        opts.Dest,
						"xver":        0,
						"serverNames": opts.ServerNames,
						"privateKey":  opts.PrivateKey,
						"shortIds":    opts.ShortIDs,
						"fingerprint": "chrome",
					},
				},
				"sniffing": map[string]any{
					"enabled":      true,
					"destOverride": []any{"http", "tls", "quic"},
				},
			},
		},
		"outbounds": []any{
			map[string]any{"tag": "direct", "protocol": "freedom", "settings": map[string]any{}},
			map[string]any{"tag": "blocked", "protocol": "blackhole", "settings": map[string]any{}},
		},
		"routing": map[string]any{
			"domainStrategy": "IPIfNonMatch",
			"rules": []any{
				map[string]any{"type": "field", "ip": []any{"geoip:private"}, "outboundTag": "blocked"},
				map[string]any{"type": "field", "protocol": []any{"bittorrent"}, "outboundTag": "blocked"},
			},
		},
	}

	raw, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("build template: %w", err)
	}
	return raw, nil
}
