package procctl

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestPathFromEnvUsesOverride(t *testing.T) {
	t.Setenv("AGENTATION_PROCCTL_PATH", "/tmp/custom-procctl.pid")

	path := PathFromEnv("AGENTATION_PROCCTL_PATH", "fallback.pid")
	if path != "/tmp/custom-procctl.pid" {
		t.Fatalf("path = %q, want override", path)
	}
}

func TestPathFromEnvUsesTempDirFallback(t *testing.T) {
	t.Setenv("AGENTATION_PROCCTL_PATH", "")

	path := PathFromEnv("AGENTATION_PROCCTL_PATH", "fallback.pid")
	want := filepath.Join(os.TempDir(), "fallback.pid")
	if path != want {
		t.Fatalf("path = %q, want %q", path, want)
	}
}

func TestControllerWriteReadAndRemovePID(t *testing.T) {
	pidPath := filepath.Join(t.TempDir(), "agentation.pid")
	ctl := New(pidPath, "")

	if err := ctl.WritePID(12345); err != nil {
		t.Fatalf("WritePID error: %v", err)
	}

	pid, err := ctl.ReadPID()
	if err != nil {
		t.Fatalf("ReadPID error: %v", err)
	}
	if pid != 12345 {
		t.Fatalf("pid = %d, want 12345", pid)
	}

	if err := ctl.RemovePIDFile(); err != nil {
		t.Fatalf("RemovePIDFile error: %v", err)
	}

	_, statErr := os.Stat(pidPath)
	if !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("expected pid file removed, stat err = %v", statErr)
	}
}

func TestControllerLoadRunningPIDRemovesStalePIDFile(t *testing.T) {
	pidPath := filepath.Join(t.TempDir(), "agentation.pid")
	if err := os.WriteFile(pidPath, []byte("0"), 0o644); err != nil {
		t.Fatalf("WriteFile error: %v", err)
	}

	ctl := New(pidPath, "")
	pid, ok := ctl.LoadRunningPID()
	if ok {
		t.Fatalf("expected not running, got pid %d", pid)
	}

	_, statErr := os.Stat(pidPath)
	if !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("expected stale pid file removed, stat err = %v", statErr)
	}
}

func TestControllerStartBackgroundCommandAndStop(t *testing.T) {
	sleepPath, err := exec.LookPath("sleep")
	if err != nil {
		t.Skip("sleep binary not available")
	}

	tempDir := t.TempDir()
	pidPath := filepath.Join(tempDir, "sleep.pid")
	logPath := filepath.Join(tempDir, "sleep.log")
	ctl := New(pidPath, "")

	pid, err := ctl.StartBackgroundCommand(sleepPath, []string{"30"}, logPath)
	if err != nil {
		t.Fatalf("StartBackgroundCommand error: %v", err)
	}

	t.Cleanup(func() {
		if IsProcessRunning(pid) {
			process, findErr := os.FindProcess(pid)
			if findErr == nil {
				_ = process.Kill()
			}
		}
		_ = ctl.RemovePIDFile()
	})

	if !IsProcessRunning(pid) {
		t.Fatalf("process %d should be running after start", pid)
	}

	stoppedPID, stopped, err := ctl.Stop(20, 50*time.Millisecond)
	if err != nil {
		t.Fatalf("Stop error: %v", err)
	}
	if !stopped {
		t.Fatal("expected process to be stopped")
	}
	if stoppedPID != pid {
		t.Fatalf("stopped pid = %d, want %d", stoppedPID, pid)
	}
	if error := waitForProcessExit(pid, time.Second); error != nil {
		t.Fatalf("waitForProcessExit returned error: %v", error)
	}
}

func waitForProcessExit(pid int, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if !IsProcessRunning(pid) {
			return nil
		}
		time.Sleep(10 * time.Millisecond)
	}
	return errors.New("process still running")
}
