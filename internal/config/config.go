// Package config loads runtime settings from the environment.
package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Panel holds every setting the panel process needs.
type Panel struct {
	// HTTPAddr serves the REST API and the embedded web UI.
	HTTPAddr string
	// GRPCAddr accepts agent control streams.
	GRPCAddr string

	DatabaseURL string
	JWTSecret   string
	TokenTTL    time.Duration

	// PublicURL is the externally reachable panel origin, used to build
	// subscription links and node installation commands.
	PublicURL string
	// SubscriptionPublicURL overrides PublicURL for subscription links when the
	// subscription page is served from a separate domain.
	SubscriptionPublicURL string

	// GRPCPublicHost/Port is what a node agent dials back on. Behind the
	// bundled Caddy this is the panel's domain on 443, and GRPCPublicTLS makes
	// the generated install command tell the agent to dial over TLS.
	GRPCPublicHost string
	GRPCPublicPort int
	GRPCPublicTLS  bool

	BootstrapAdmin    string
	BootstrapPassword string

	HeartbeatInterval time.Duration
	UsageInterval     time.Duration
	UsageRetention    time.Duration

	SubscriptionProfileTitle string
	SubscriptionSupportURL   string

	// AgentDistDir holds prebuilt node agent binaries the panel serves at
	// /dist, so a node can install itself without a Go toolchain or a registry.
	AgentDistDir string

	CORSOrigins []string
	LogLevel    string
	TLSCert     string
	TLSKey      string
}

func LoadPanel() (*Panel, error) {
	p := &Panel{
		HTTPAddr:                 env("PANEL_HTTP_ADDR", ":8080"),
		GRPCAddr:                 env("PANEL_GRPC_ADDR", ":9090"),
		DatabaseURL:              env("DATABASE_URL", ""),
		JWTSecret:                env("JWT_SECRET", ""),
		TokenTTL:                 envDuration("JWT_TTL", 24*time.Hour),
		PublicURL:                strings.TrimRight(env("PANEL_PUBLIC_URL", "http://localhost:8080"), "/"),
		SubscriptionPublicURL:    strings.TrimRight(env("SUBSCRIPTION_PUBLIC_URL", ""), "/"),
		GRPCPublicHost:           env("PANEL_GRPC_PUBLIC_HOST", ""),
		GRPCPublicPort:           envInt("PANEL_GRPC_PUBLIC_PORT", 9090),
		GRPCPublicTLS:            envBool("PANEL_GRPC_PUBLIC_TLS", false),
		BootstrapAdmin:           env("PANEL_ADMIN_USERNAME", "admin"),
		BootstrapPassword:        env("PANEL_ADMIN_PASSWORD", ""),
		HeartbeatInterval:        envDuration("NODE_HEARTBEAT_INTERVAL", 10*time.Second),
		UsageInterval:            envDuration("NODE_USAGE_INTERVAL", 30*time.Second),
		UsageRetention:           envDuration("USAGE_RETENTION", 90*24*time.Hour),
		SubscriptionProfileTitle: env("SUBSCRIPTION_TITLE", "AmneziaX"),
		SubscriptionSupportURL:   env("SUBSCRIPTION_SUPPORT_URL", ""),
		AgentDistDir:             env("AGENT_DIST_DIR", "/usr/local/share/amneziax/dist"),
		CORSOrigins:              splitList(env("CORS_ORIGINS", "*")),
		LogLevel:                 env("LOG_LEVEL", "info"),
		TLSCert:                  env("PANEL_TLS_CERT", ""),
		TLSKey:                   env("PANEL_TLS_KEY", ""),
	}
	if p.DatabaseURL == "" {
		return nil, fmt.Errorf("DATABASE_URL is required")
	}
	if p.JWTSecret == "" {
		return nil, fmt.Errorf("JWT_SECRET is required")
	}
	if p.SubscriptionPublicURL == "" {
		p.SubscriptionPublicURL = p.PublicURL
	}
	return p, nil
}

// Agent holds the node agent settings.
type Agent struct {
	PanelAddr string
	NodeUUID  string
	Token     string
	Insecure  bool
	// ServerName overrides the TLS SNI when the panel sits behind a proxy.
	ServerName string

	XrayBinary     string
	HysteriaBinary string
	SingBoxBinary  string
	XrayWorkDir    string
	// XrayAPIAddr is the local address of the xray stats API injected into the
	// generated configuration.
	XrayAPIAddr string

	LogLevel string
}

func LoadAgent() (*Agent, error) {
	a := &Agent{
		PanelAddr:      env("PANEL_GRPC_ADDR", ""),
		NodeUUID:       env("NODE_UUID", ""),
		Token:          env("NODE_TOKEN", ""),
		Insecure:       envBool("PANEL_GRPC_INSECURE", true),
		ServerName:     env("PANEL_GRPC_SERVER_NAME", ""),
		XrayBinary:     env("XRAY_BINARY", "/usr/local/bin/xray"),
		HysteriaBinary: env("HYSTERIA_BINARY", "/usr/local/bin/hysteria"),
		SingBoxBinary:  env("SINGBOX_BINARY", "/usr/local/bin/sing-box"),
		XrayWorkDir:    env("XRAY_WORKDIR", "/var/lib/amneziax-node"),
		XrayAPIAddr:    env("XRAY_API_ADDR", "127.0.0.1:10085"),
		LogLevel:       env("LOG_LEVEL", "info"),
	}
	if a.PanelAddr == "" {
		return nil, fmt.Errorf("PANEL_GRPC_ADDR is required")
	}
	if a.NodeUUID == "" {
		return nil, fmt.Errorf("NODE_UUID is required")
	}
	if a.Token == "" {
		return nil, fmt.Errorf("NODE_TOKEN is required")
	}
	return a, nil
}

func env(key, def string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return def
}

func envInt(key string, def int) int {
	if v, err := strconv.Atoi(env(key, "")); err == nil {
		return v
	}
	return def
}

func envBool(key string, def bool) bool {
	if v, err := strconv.ParseBool(env(key, "")); err == nil {
		return v
	}
	return def
}

func envDuration(key string, def time.Duration) time.Duration {
	raw := env(key, "")
	if raw == "" {
		return def
	}
	if d, err := time.ParseDuration(raw); err == nil {
		return d
	}
	if secs, err := strconv.Atoi(raw); err == nil {
		return time.Duration(secs) * time.Second
	}
	return def
}

func splitList(raw string) []string {
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}
