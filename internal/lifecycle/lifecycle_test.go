package lifecycle

import (
	"bytes"
	"testing"
)

func TestParseStartFlags_DefaultServerOnly(t *testing.T) {
	t.Setenv("AGENTATION_SERVER_ADDR", "")
	t.Setenv("AGENTATION_ROUTER_ADDR", "")

	cfg, err := parseStartFlags(nil, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("parseStartFlags error: %v", err)
	}

	if !cfg.server {
		t.Fatalf("server should be enabled by default")
	}
	if cfg.router {
		t.Fatalf("router should be disabled by default")
	}
	if cfg.serverAddr != defaultServerAddress {
		t.Fatalf("server addr = %q, want %q", cfg.serverAddr, defaultServerAddress)
	}
}

func TestParseStartFlags_ServerAddrFromEnv(t *testing.T) {
	t.Setenv("AGENTATION_SERVER_ADDR", "127.0.0.1:5757")
	t.Setenv("AGENTATION_ROUTER_ADDR", "")

	cfg, err := parseStartFlags(nil, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("parseStartFlags error: %v", err)
	}
	if cfg.serverAddr != "127.0.0.1:5757" {
		t.Fatalf("server addr = %q, want env value", cfg.serverAddr)
	}
}

func TestParseStartFlags_EnableRouterFromEnv(t *testing.T) {
	t.Setenv("AGENTATION_SERVER_ADDR", "")
	t.Setenv("AGENTATION_ROUTER_ADDR", "127.0.0.1:9999")

	cfg, err := parseStartFlags(nil, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("parseStartFlags error: %v", err)
	}

	if !cfg.server || !cfg.router {
		t.Fatalf("expected server and router enabled, got server=%v router=%v", cfg.server, cfg.router)
	}
	if cfg.routerAddr != "127.0.0.1:9999" {
		t.Fatalf("router addr = %q, want env value", cfg.routerAddr)
	}
}

func TestParseStartFlags_ExplicitSelection(t *testing.T) {
	t.Setenv("AGENTATION_ROUTER_ADDR", "")

	cfg, err := parseStartFlags([]string{"--router", "--router-addr", "127.0.0.1:8788"}, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("parseStartFlags error: %v", err)
	}

	if cfg.server {
		t.Fatalf("server should be disabled when only router is selected")
	}
	if !cfg.router {
		t.Fatalf("router should be enabled")
	}
	if cfg.routerAddr != "127.0.0.1:8788" {
		t.Fatalf("router addr = %q, want 127.0.0.1:8788", cfg.routerAddr)
	}
}

func TestParseStartFlags_ConflictingModeFlags(t *testing.T) {
	_, err := parseStartFlags([]string{"--foreground", "--background"}, &bytes.Buffer{})
	if err == nil {
		t.Fatal("expected error for conflicting mode flags")
	}
}

func TestParseControlFlags_DefaultSelection(t *testing.T) {
	t.Setenv("AGENTATION_ROUTER_ADDR", "")

	cfg, err := parseControlFlags("status", nil, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("parseControlFlags error: %v", err)
	}
	if !cfg.server || cfg.router {
		t.Fatalf("expected default selection server=true router=false, got %+v", cfg)
	}

	t.Setenv("AGENTATION_ROUTER_ADDR", "127.0.0.1:8787")
	cfg, err = parseControlFlags("status", nil, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("parseControlFlags error: %v", err)
	}
	if !cfg.server || !cfg.router {
		t.Fatalf("expected default selection server=true router=true when env set, got %+v", cfg)
	}
}

func TestParseStartFlags_RouterAddrFlagOverridesEnv(t *testing.T) {
	t.Setenv("AGENTATION_ROUTER_ADDR", "127.0.0.1:9999")

	cfg, err := parseStartFlags([]string{"--router", "--router-addr", "127.0.0.1:8788"}, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("parseStartFlags error: %v", err)
	}
	if cfg.routerAddr != "127.0.0.1:8788" {
		t.Fatalf("router addr = %q, want flag override", cfg.routerAddr)
	}
}

func TestParseStartFlags_ServerAddrFlagOverridesEnv(t *testing.T) {
	t.Setenv("AGENTATION_SERVER_ADDR", "127.0.0.1:5757")
	t.Setenv("AGENTATION_ROUTER_ADDR", "")

	cfg, err := parseStartFlags([]string{"--server", "--server-addr", "127.0.0.1:4748"}, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("parseStartFlags error: %v", err)
	}
	if cfg.serverAddr != "127.0.0.1:4748" {
		t.Fatalf("server addr = %q, want flag override", cfg.serverAddr)
	}
}
