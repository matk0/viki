package config_test

import (
	"testing"
	"time"

	"viki/internal/config"
)

func TestLoadUsesSafeLocalDefaults(t *testing.T) {
	clearConfigEnvironment(t)
	t.Setenv("INITIAL_USER_PASSWORD", "password")

	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}

	if cfg.Address != ":8080" || cfg.InternalAddress != "127.0.0.1:8090" {
		t.Fatalf("unexpected listen addresses: public=%q internal=%q", cfg.Address, cfg.InternalAddress)
	}
	if cfg.DatabaseURL != "postgres://viki:viki@localhost:5432/viki?sslmode=disable" {
		t.Fatalf("unexpected database URL %q", cfg.DatabaseURL)
	}
	if cfg.HermesQAWSURL != "ws://127.0.0.1:9119/api/ws" || cfg.HermesEditWSURL != "ws://127.0.0.1:9120/api/ws" {
		t.Fatalf("unexpected Hermes URLs: qa=%q edit=%q", cfg.HermesQAWSURL, cfg.HermesEditWSURL)
	}
	if cfg.CookieSecure || cfg.HermesQAConfigured || cfg.HermesEditConfigured || cfg.DeveloperEnabled || cfg.DeveloperToolToken != "" {
		t.Fatalf("optional flags must default off: %+v", cfg)
	}
	if cfg.SessionTTL != 12*time.Hour || cfg.FrontendDir != "../frontend/dist" {
		t.Fatalf("unexpected local defaults: ttl=%s frontend=%q", cfg.SessionTTL, cfg.FrontendDir)
	}
	if cfg.DevelopmentTargetURL != "http://127.0.0.1:8091" || cfg.DevelopmentTargetToken != "" {
		t.Fatalf("unexpected development target defaults: url=%q token=%q", cfg.DevelopmentTargetURL, cfg.DevelopmentTargetToken)
	}
}

func TestLoadReadsEverySupportedOverride(t *testing.T) {
	clearConfigEnvironment(t)
	overrides := map[string]string{
		"VIKI_ADDRESS":              "localhost:8081",
		"VIKI_INTERNAL_ADDRESS":     "[::1]:8091",
		"DATABASE_URL":              "postgres://example.test/viki",
		"INITIAL_USER_PASSWORD":     "secret",
		"HERMES_QA_WS_URL":          "ws://localhost:9219/ws",
		"HERMES_EDIT_WS_URL":        "ws://localhost:9220/ws",
		"HERMES_QA_TOKEN":           "qa-token",
		"HERMES_EDIT_TOKEN":         "edit-token",
		"HERMES_QA_CONFIGURED":      "true",
		"HERMES_EDIT_CONFIGURED":    "not-a-boolean",
		"VIKI_HERMES_TOOL_TOKEN":    "tool-token",
		"VIKI_DEVELOPER_ENABLED":    "true",
		"VIKI_DEVELOPER_TOOL_TOKEN": "developer-token",
		"COOKIE_SECURE":             "true",
		"FRONTEND_DIR":              "/srv/viki/public",
		"DEVELOPMENT_TARGET_URL":    "http://mock-target:8091",
		"DEVELOPMENT_TARGET_TOKEN":  "target-token",
	}
	for name, value := range overrides {
		t.Setenv(name, value)
	}

	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}

	if cfg.Address != overrides["VIKI_ADDRESS"] || cfg.InternalAddress != overrides["VIKI_INTERNAL_ADDRESS"] {
		t.Fatalf("listen overrides were not loaded: %+v", cfg)
	}
	if cfg.DatabaseURL != overrides["DATABASE_URL"] || cfg.InitialPassword != "secret" {
		t.Fatalf("core overrides were not loaded: %+v", cfg)
	}
	if cfg.HermesQAToken != "qa-token" || cfg.HermesEditToken != "edit-token" || cfg.HermesToolToken != "tool-token" || cfg.DeveloperToolToken != "developer-token" {
		t.Fatalf("Hermes secrets were not loaded")
	}
	if !cfg.HermesQAConfigured || cfg.HermesEditConfigured || !cfg.CookieSecure || !cfg.DeveloperEnabled {
		t.Fatalf("boolean overrides were not parsed safely: %+v", cfg)
	}
	if cfg.FrontendDir != "/srv/viki/public" {
		t.Fatalf("frontend override = %q", cfg.FrontendDir)
	}
	if cfg.DevelopmentTargetURL != "http://mock-target:8091" || cfg.DevelopmentTargetToken != "target-token" {
		t.Fatalf("development target overrides were not loaded: %+v", cfg)
	}
}

func TestLoadRequiresPasswordAndSafeInternalListener(t *testing.T) {
	clearConfigEnvironment(t)
	if _, err := config.Load(); err == nil {
		t.Fatal("missing initial password was accepted")
	}

	invalidAddresses := []string{"missing-port", "localhost:8090", "192.0.2.1:8090"}
	for _, address := range invalidAddresses {
		t.Run(address, func(t *testing.T) {
			clearConfigEnvironment(t)
			t.Setenv("INITIAL_USER_PASSWORD", "password")
			t.Setenv("VIKI_INTERNAL_ADDRESS", address)
			if _, err := config.Load(); err == nil {
				t.Fatalf("unsafe internal address %q was accepted", address)
			}
		})
	}
}

func TestLoadAllowsAuthenticatedPrivateNetworkInternalListener(t *testing.T) {
	clearConfigEnvironment(t)
	t.Setenv("INITIAL_USER_PASSWORD", "password")
	t.Setenv("VIKI_INTERNAL_ADDRESS", "0.0.0.0:8090")
	t.Setenv("VIKI_HERMES_TOOL_TOKEN", "service-token")

	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.InternalAddress != "0.0.0.0:8090" {
		t.Fatalf("internal address = %q", cfg.InternalAddress)
	}

	t.Setenv("VIKI_HERMES_TOOL_TOKEN", "")
	if _, err := config.Load(); err == nil {
		t.Fatal("wildcard internal listener without service authentication was accepted")
	}
}

func TestLoadRequiresExplicitDeveloperCapabilityAndTargetCredential(t *testing.T) {
	clearConfigEnvironment(t)
	t.Setenv("INITIAL_USER_PASSWORD", "password")
	t.Setenv("VIKI_DEVELOPER_ENABLED", "true")
	if _, err := config.Load(); err == nil {
		t.Fatal("developer execution without a capability was accepted")
	}
	t.Setenv("VIKI_DEVELOPER_TOOL_TOKEN", "developer-token")
	if _, err := config.Load(); err == nil {
		t.Fatal("developer execution without a target credential was accepted")
	}
	t.Setenv("DEVELOPMENT_TARGET_TOKEN", "target-token")
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.DeveloperEnabled || cfg.DeveloperToolToken != "developer-token" {
		t.Fatalf("developer capability was not loaded: %+v", cfg)
	}
}

func clearConfigEnvironment(t *testing.T) {
	t.Helper()
	for _, name := range []string{
		"VIKI_ADDRESS",
		"VIKI_INTERNAL_ADDRESS",
		"DATABASE_URL",
		"INITIAL_USER_PASSWORD",
		"HERMES_QA_WS_URL",
		"HERMES_EDIT_WS_URL",
		"HERMES_QA_TOKEN",
		"HERMES_EDIT_TOKEN",
		"HERMES_QA_CONFIGURED",
		"HERMES_EDIT_CONFIGURED",
		"VIKI_HERMES_TOOL_TOKEN",
		"VIKI_DEVELOPER_ENABLED",
		"VIKI_DEVELOPER_TOOL_TOKEN",
		"COOKIE_SECURE",
		"FRONTEND_DIR",
		"DEVELOPMENT_TARGET_URL",
		"DEVELOPMENT_TARGET_TOKEN",
	} {
		t.Setenv(name, "")
	}
}
