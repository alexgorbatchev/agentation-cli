package lifecycle

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestResolveServeConfig_DefaultStartsBoth(t *testing.T) {
	t.Setenv("AGENTATION_SERVER_ADDR", "")
	t.Setenv("AGENTATION_ROUTER_ADDR", "")

	cfg, err := resolveServeConfig("", "")
	if err != nil {
		t.Fatalf("resolveServeConfig error: %v", err)
	}

	if !cfg.enableServer || !cfg.enableRouter {
		t.Fatalf("expected both services enabled, got server=%v router=%v", cfg.enableServer, cfg.enableRouter)
	}
	if cfg.serverAddr != defaultServerAddress {
		t.Fatalf("server addr = %q, want %q", cfg.serverAddr, defaultServerAddress)
	}
	if cfg.routerAddr != defaultRouterAddress {
		t.Fatalf("router addr = %q, want %q", cfg.routerAddr, defaultRouterAddress)
	}
}

func TestResolveServeConfig_DisableServerWithZero(t *testing.T) {
	t.Setenv("AGENTATION_SERVER_ADDR", "0")
	t.Setenv("AGENTATION_ROUTER_ADDR", "")

	cfg, err := resolveServeConfig("", "")
	if err != nil {
		t.Fatalf("resolveServeConfig error: %v", err)
	}
	if cfg.enableServer {
		t.Fatal("server should be disabled when AGENTATION_SERVER_ADDR=0")
	}
	if !cfg.enableRouter {
		t.Fatal("router should remain enabled")
	}
}

func TestResolveServeConfig_DisableRouterWithZero(t *testing.T) {
	t.Setenv("AGENTATION_SERVER_ADDR", "")
	t.Setenv("AGENTATION_ROUTER_ADDR", "0")

	cfg, err := resolveServeConfig("", "")
	if err != nil {
		t.Fatalf("resolveServeConfig error: %v", err)
	}
	if !cfg.enableServer {
		t.Fatal("server should remain enabled")
	}
	if cfg.enableRouter {
		t.Fatal("router should be disabled when AGENTATION_ROUTER_ADDR=0")
	}
}

func TestResolveServeConfig_BothDisabledErrors(t *testing.T) {
	t.Setenv("AGENTATION_SERVER_ADDR", "0")
	t.Setenv("AGENTATION_ROUTER_ADDR", "0")

	if _, err := resolveServeConfig("", ""); err == nil {
		t.Fatal("expected error when both services are disabled")
	}
}

func TestResolveServeConfig_FlagOverridesEnv(t *testing.T) {
	t.Setenv("AGENTATION_SERVER_ADDR", "0")
	t.Setenv("AGENTATION_ROUTER_ADDR", "0")

	cfg, err := resolveServeConfig("127.0.0.1:4748", "127.0.0.1:8788")
	if err != nil {
		t.Fatalf("resolveServeConfig error: %v", err)
	}

	if !cfg.enableServer || !cfg.enableRouter {
		t.Fatalf("expected both services enabled, got server=%v router=%v", cfg.enableServer, cfg.enableRouter)
	}
	if cfg.serverAddr != "127.0.0.1:4748" {
		t.Fatalf("server addr = %q, want flag override", cfg.serverAddr)
	}
	if cfg.routerAddr != "127.0.0.1:8788" {
		t.Fatalf("router addr = %q, want flag override", cfg.routerAddr)
	}
}

func TestResolveServeConfig_RouterLegacyEnvFallback(t *testing.T) {
	t.Setenv("AGENTATION_SERVER_ADDR", "")
	t.Setenv("AGENTATION_ROUTER_ADDR", "")
	t.Setenv("AGENTATION_ROUTER_ADDRESS", "127.0.0.1:8999")

	cfg, err := resolveServeConfig("", "")
	if err != nil {
		t.Fatalf("resolveServeConfig error: %v", err)
	}
	if cfg.routerAddr != "127.0.0.1:8999" {
		t.Fatalf("router addr = %q, want legacy env fallback", cfg.routerAddr)
	}
}

func TestParseStartFlags_ConflictingModeFlags(t *testing.T) {
	_, err := parseStartFlags([]string{"--foreground", "--background"}, &bytes.Buffer{})
	if err == nil {
		t.Fatal("expected error for conflicting mode flags")
	}
}

func TestParseNoArgCommandRejectsPositional(t *testing.T) {
	err := parseNoArgCommand("status", []string{"extra"}, &bytes.Buffer{})
	if err == nil {
		t.Fatal("expected positional argument error")
	}
}

func installFakePgrep(t *testing.T) string {
	t.Helper()

	binDir := t.TempDir()
	markerPath := filepath.Join(binDir, "pgrep-called")
	pgrepPath := filepath.Join(binDir, "pgrep")
	script := "#!/bin/sh\nprintf called > \"$AGENTATION_TEST_SCAN_MARKER\"\nexit 1\n"
	if err := os.WriteFile(pgrepPath, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake pgrep: %v", err)
	}

	currentPath := os.Getenv("PATH")
	t.Setenv("AGENTATION_TEST_SCAN_MARKER", markerPath)
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+currentPath)
	return markerPath
}

func TestLoadRunningPID_SkipsGlobalScanWhenPIDFilePathIsExplicit(t *testing.T) {
	markerPath := installFakePgrep(t)
	pidPath := filepath.Join(t.TempDir(), "agentation.pid")
	t.Setenv("AGENTATION_PID_FILE", pidPath)

	if pid, ok := loadRunningPID(); ok || pid != 0 {
		t.Fatalf("loadRunningPID() = (%d, %v), want (0, false)", pid, ok)
	}

	if _, err := os.Stat(markerPath); !os.IsNotExist(err) {
		t.Fatalf("expected explicit AGENTATION_PID_FILE to skip global pgrep scan, stat err = %v", err)
	}
}

func TestLoadRunningPID_UsesGlobalScanWithDefaultPIDPath(t *testing.T) {
	markerPath := installFakePgrep(t)
	t.Setenv("AGENTATION_PID_FILE", "")
	t.Setenv("TMPDIR", t.TempDir())

	if pid, ok := loadRunningPID(); ok || pid != 0 {
		t.Fatalf("loadRunningPID() = (%d, %v), want (0, false)", pid, ok)
	}

	if _, err := os.Stat(markerPath); err != nil {
		t.Fatalf("expected default lifecycle lookup to run pgrep fallback, stat err = %v", err)
	}
}
