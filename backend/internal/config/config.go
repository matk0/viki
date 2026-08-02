package config

import (
	"fmt"
	"net"
	"os"
	"strconv"
	"time"
)

type Config struct {
	Address                string
	InternalAddress        string
	DatabaseURL            string
	InitialPassword        string
	HermesQAWSURL          string
	HermesEditWSURL        string
	HermesQAToken          string
	HermesEditToken        string
	HermesQAConfigured     bool
	HermesEditConfigured   bool
	HermesToolToken        string
	CookieSecure           bool
	SessionTTL             time.Duration
	FrontendDir            string
	DevelopmentTargetURL   string
	DevelopmentTargetToken string
}

func Load() (Config, error) {
	cfg := Config{
		Address:                env("VIKI_ADDRESS", ":8080"),
		InternalAddress:        env("VIKI_INTERNAL_ADDRESS", "127.0.0.1:8090"),
		DatabaseURL:            env("DATABASE_URL", "postgres://viki:viki@localhost:5432/viki?sslmode=disable"),
		InitialPassword:        os.Getenv("INITIAL_USER_PASSWORD"),
		HermesQAWSURL:          env("HERMES_QA_WS_URL", "ws://127.0.0.1:9119/api/ws"),
		HermesEditWSURL:        env("HERMES_EDIT_WS_URL", "ws://127.0.0.1:9120/api/ws"),
		HermesQAToken:          os.Getenv("HERMES_QA_TOKEN"),
		HermesEditToken:        os.Getenv("HERMES_EDIT_TOKEN"),
		HermesQAConfigured:     envBool("HERMES_QA_CONFIGURED", false),
		HermesEditConfigured:   envBool("HERMES_EDIT_CONFIGURED", false),
		HermesToolToken:        os.Getenv("VIKI_HERMES_TOOL_TOKEN"),
		CookieSecure:           envBool("COOKIE_SECURE", false),
		SessionTTL:             12 * time.Hour,
		FrontendDir:            env("FRONTEND_DIR", "../frontend/dist"),
		DevelopmentTargetURL:   env("DEVELOPMENT_TARGET_URL", "http://127.0.0.1:8091"),
		DevelopmentTargetToken: os.Getenv("DEVELOPMENT_TARGET_TOKEN"),
	}
	if cfg.InitialPassword == "" {
		return Config{}, fmt.Errorf("INITIAL_USER_PASSWORD is required")
	}
	host, _, err := net.SplitHostPort(cfg.InternalAddress)
	ip := net.ParseIP(host)
	if err != nil || ip == nil || (!ip.IsLoopback() && !ip.IsUnspecified()) {
		return Config{}, fmt.Errorf("VIKI_INTERNAL_ADDRESS must bind to an explicit loopback or wildcard address")
	}
	if ip.IsUnspecified() && cfg.HermesToolToken == "" {
		return Config{}, fmt.Errorf("VIKI_HERMES_TOOL_TOKEN is required for a wildcard internal listener")
	}
	return cfg, nil
}

func env(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}

func envBool(name string, fallback bool) bool {
	value := os.Getenv(name)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return fallback
	}
	return parsed
}
