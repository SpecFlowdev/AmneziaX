// Package xray parses and renders xray-core configuration documents.
package xray

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/SpecFlowdev/AmneziaX/internal/domain"
)

// Document is the subset of an xray-core config the panel needs to reason
// about. Everything else is preserved verbatim through Extra.
type Document struct {
	Log       json.RawMessage   `json:"log,omitempty"`
	DNS       json.RawMessage   `json:"dns,omitempty"`
	Inbounds  []json.RawMessage `json:"inbounds"`
	Outbounds []json.RawMessage `json:"outbounds,omitempty"`
	Routing   *Routing          `json:"routing,omitempty"`
	Policy    json.RawMessage   `json:"policy,omitempty"`
	API       json.RawMessage   `json:"api,omitempty"`
	Stats     json.RawMessage   `json:"stats,omitempty"`
	Transport json.RawMessage   `json:"transport,omitempty"`
	Reverse   json.RawMessage   `json:"reverse,omitempty"`
	FakeDNS   json.RawMessage   `json:"fakedns,omitempty"`
	Metrics   json.RawMessage   `json:"metrics,omitempty"`
	Observ    json.RawMessage   `json:"observatory,omitempty"`
	Burst     json.RawMessage   `json:"burstObservatory,omitempty"`
}

type Routing struct {
	DomainStrategy string            `json:"domainStrategy,omitempty"`
	DomainMatcher  string            `json:"domainMatcher,omitempty"`
	Rules          []json.RawMessage `json:"rules,omitempty"`
	Balancers      []json.RawMessage `json:"balancers,omitempty"`
}

// inboundShape is what we read out of each inbound entry.
type inboundShape struct {
	Tag            string          `json:"tag"`
	Protocol       string          `json:"protocol"`
	Port           any             `json:"port"`
	Listen         string          `json:"listen,omitempty"`
	Settings       json.RawMessage `json:"settings,omitempty"`
	StreamSettings *struct {
		Network  string `json:"network"`
		Security string `json:"security"`
	} `json:"streamSettings,omitempty"`
}

// ErrNoInbounds signals a profile document that cannot serve any traffic.
var ErrNoInbounds = fmt.Errorf("configuration has no inbounds")

// Validate checks that a profile document is well formed and every inbound
// carries a unique, non-empty tag. Tags are the join key between profiles,
// nodes, squads and hosts, so a missing tag is a hard error rather than a
// warning.
func Validate(raw json.RawMessage) error {
	var doc Document
	if err := json.Unmarshal(raw, &doc); err != nil {
		return fmt.Errorf("invalid JSON: %w", err)
	}
	if len(doc.Inbounds) == 0 {
		return ErrNoInbounds
	}
	seen := map[string]bool{}
	for i, raw := range doc.Inbounds {
		var in inboundShape
		if err := json.Unmarshal(raw, &in); err != nil {
			return fmt.Errorf("inbound #%d: %w", i+1, err)
		}
		if strings.TrimSpace(in.Tag) == "" {
			return fmt.Errorf("inbound #%d has no tag", i+1)
		}
		if in.Tag == apiInboundTag {
			return fmt.Errorf("inbound tag %q is reserved by the panel", apiInboundTag)
		}
		if seen[in.Tag] {
			return fmt.Errorf("duplicate inbound tag %q", in.Tag)
		}
		seen[in.Tag] = true
		if strings.TrimSpace(in.Protocol) == "" {
			return fmt.Errorf("inbound %q has no protocol", in.Tag)
		}
	}
	if len(doc.Outbounds) == 0 {
		return fmt.Errorf("configuration has no outbounds")
	}
	return nil
}

// ParseInbounds extracts the inbound summary rows persisted alongside a profile.
func ParseInbounds(raw json.RawMessage) ([]domain.ConfigProfileInbound, error) {
	var doc Document
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, err
	}
	out := make([]domain.ConfigProfileInbound, 0, len(doc.Inbounds))
	for _, item := range doc.Inbounds {
		var in inboundShape
		if err := json.Unmarshal(item, &in); err != nil {
			return nil, err
		}
		if in.Tag == "" || in.Tag == apiInboundTag {
			continue
		}
		summary := domain.ConfigProfileInbound{
			Tag:  in.Tag,
			Type: strings.ToLower(in.Protocol),
			Port: portOf(in.Port),
		}
		if in.StreamSettings != nil {
			summary.Network = in.StreamSettings.Network
			summary.Security = in.StreamSettings.Security
		}
		if summary.Network == "" {
			summary.Network = "tcp"
		}
		if summary.Security == "" {
			summary.Security = "none"
		}
		out = append(out, summary)
	}
	return out, nil
}

func portOf(v any) int {
	switch t := v.(type) {
	case float64:
		return int(t)
	case string:
		// Port ranges such as "10000-20000" report their lower bound.
		var p int
		if _, err := fmt.Sscanf(t, "%d", &p); err == nil {
			return p
		}
	}
	return 0
}

// Client is a user entry injected into an inbound.
type Client struct {
	Email      string
	VlessUUID  string
	TrojanPass string
	SSPass     string
	Flow       string

	// WireGuard identifies a peer by its public key and the address it may use
	// inside the tunnel. The private half never leaves the panel — a node only
	// ever needs the public key.
	WGPublicKey string
	WGAddress   string
}

const (
	apiInboundTag = "amneziax-api"
	apiOutboundTg = "amneziax-api-out"
)

// RenderOptions controls how a node-specific config is produced from a profile.
type RenderOptions struct {
	// ActiveTags restricts the rendered document to these inbounds. Empty means
	// every inbound of the profile.
	ActiveTags []string
	// ClientsByTag maps an inbound tag to the users allowed to use it.
	ClientsByTag map[string][]Client
	// APIListen is the address the stats API listens on inside the node.
	APIListen string
	APIPort   int
}

// Render produces the exact configuration a node should run, together with a
// hash that lets the agent skip a restart when nothing changed.
func Render(profile json.RawMessage, opts RenderOptions) ([]byte, string, error) {
	var doc Document
	if err := json.Unmarshal(profile, &doc); err != nil {
		return nil, "", fmt.Errorf("invalid profile config: %w", err)
	}

	active := map[string]bool{}
	for _, t := range opts.ActiveTags {
		active[t] = true
	}

	rendered := make([]json.RawMessage, 0, len(doc.Inbounds)+1)
	for _, item := range doc.Inbounds {
		var in inboundShape
		if err := json.Unmarshal(item, &in); err != nil {
			return nil, "", err
		}
		if in.Tag == "" || in.Tag == apiInboundTag {
			continue
		}
		if len(active) > 0 && !active[in.Tag] {
			continue
		}
		patched, err := injectClients(item, strings.ToLower(in.Protocol), opts.ClientsByTag[in.Tag])
		if err != nil {
			return nil, "", fmt.Errorf("inbound %q: %w", in.Tag, err)
		}
		rendered = append(rendered, patched)
	}
	if len(rendered) == 0 {
		return nil, "", ErrNoInbounds
	}

	listen := opts.APIListen
	if listen == "" {
		listen = "127.0.0.1"
	}
	port := opts.APIPort
	if port == 0 {
		port = 10085
	}

	apiInbound, err := json.Marshal(map[string]any{
		"tag":      apiInboundTag,
		"listen":   listen,
		"port":     port,
		"protocol": "dokodemo-door",
		"settings": map[string]any{"address": listen},
	})
	if err != nil {
		return nil, "", err
	}
	doc.Inbounds = append([]json.RawMessage{apiInbound}, rendered...)

	// The stats API needs a matching service definition, a policy that records
	// per-user counters, and a routing rule that keeps its inbound internal.
	doc.API = json.RawMessage(fmt.Sprintf(`{"tag":%q,"services":["HandlerService","StatsService","LoggerService"]}`, apiOutboundTg))
	doc.Stats = json.RawMessage(`{}`)
	doc.Policy = json.RawMessage(`{"levels":{"0":{"statsUserUplink":true,"statsUserDownlink":true,"handshake":4,"connIdle":300,"uplinkOnly":2,"downlinkOnly":4,"bufferSize":4}},"system":{"statsInboundUplink":true,"statsInboundDownlink":true,"statsOutboundUplink":true,"statsOutboundDownlink":true}}`)

	apiRule := json.RawMessage(fmt.Sprintf(`{"type":"field","inboundTag":[%q],"outboundTag":%q}`, apiInboundTag, apiOutboundTg))
	if doc.Routing == nil {
		doc.Routing = &Routing{}
	}
	doc.Routing.Rules = append([]json.RawMessage{apiRule}, doc.Routing.Rules...)

	out, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return nil, "", err
	}
	sum := sha256.Sum256(out)
	return out, hex.EncodeToString(sum[:]), nil
}

// injectClients rewrites settings.clients (or settings.password for
// shadowsocks) so the inbound serves exactly the given users.
func injectClients(raw json.RawMessage, protocol string, clients []Client) (json.RawMessage, error) {
	var inbound map[string]json.RawMessage
	if err := json.Unmarshal(raw, &inbound); err != nil {
		return nil, err
	}
	settings := map[string]json.RawMessage{}
	if s, ok := inbound["settings"]; ok && len(s) > 0 {
		if err := json.Unmarshal(s, &settings); err != nil {
			return nil, fmt.Errorf("settings: %w", err)
		}
	}

	switch protocol {
	case "vless":
		entries := make([]map[string]any, 0, len(clients))
		for _, c := range clients {
			e := map[string]any{"id": c.VlessUUID, "email": c.Email, "level": 0}
			if c.Flow != "" {
				e["flow"] = c.Flow
			}
			entries = append(entries, e)
		}
		if err := setJSON(settings, "clients", entries); err != nil {
			return nil, err
		}
		if _, ok := settings["decryption"]; !ok {
			settings["decryption"] = json.RawMessage(`"none"`)
		}
	case "vmess":
		entries := make([]map[string]any, 0, len(clients))
		for _, c := range clients {
			entries = append(entries, map[string]any{"id": c.VlessUUID, "email": c.Email, "level": 0})
		}
		if err := setJSON(settings, "clients", entries); err != nil {
			return nil, err
		}
	case "trojan":
		entries := make([]map[string]any, 0, len(clients))
		for _, c := range clients {
			entries = append(entries, map[string]any{"password": c.TrojanPass, "email": c.Email, "level": 0})
		}
		if err := setJSON(settings, "clients", entries); err != nil {
			return nil, err
		}
	case "shadowsocks":
		entries := make([]map[string]any, 0, len(clients))
		for _, c := range clients {
			entries = append(entries, map[string]any{"password": c.SSPass, "email": c.Email, "level": 0})
		}
		if err := setJSON(settings, "clients", entries); err != nil {
			return nil, err
		}
	case "wireguard":
		// A peer list, not a client list. A user without a key is skipped
		// rather than written as an empty peer: xray rejects that outright,
		// which would take the inbound down for everybody else too.
		entries := make([]map[string]any, 0, len(clients))
		for _, c := range clients {
			if c.WGPublicKey == "" || c.WGAddress == "" {
				continue
			}
			entries = append(entries, map[string]any{
				"publicKey":  c.WGPublicKey,
				"allowedIPs": []string{c.WGAddress},
			})
		}
		if err := setJSON(settings, "peers", entries); err != nil {
			return nil, err
		}
	default:
		// Protocols without per-user identities (socks, http, dokodemo-door…)
		// are passed through untouched.
		return raw, nil
	}

	encoded, err := json.Marshal(settings)
	if err != nil {
		return nil, err
	}
	inbound["settings"] = encoded
	return json.Marshal(inbound)
}

func setJSON(m map[string]json.RawMessage, key string, value any) error {
	encoded, err := json.Marshal(value)
	if err != nil {
		return err
	}
	m[key] = encoded
	return nil
}

// SortedTags returns the inbound tags of a document in a stable order.
func SortedTags(raw json.RawMessage) []string {
	inbounds, err := ParseInbounds(raw)
	if err != nil {
		return nil
	}
	tags := make([]string, 0, len(inbounds))
	for _, in := range inbounds {
		tags = append(tags, in.Tag)
	}
	sort.Strings(tags)
	return tags
}
