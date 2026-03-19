package procctl

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const (
	defaultStopWaitAttempts = 30
	defaultStopWaitDelay    = 100 * time.Millisecond
	startProbeDelay         = 250 * time.Millisecond
)

type Controller struct {
	PIDFilePath string
	ScanMarker  string
}

func New(pidFilePath string, scanMarker string) Controller {
	return Controller{
		PIDFilePath: pidFilePath,
		ScanMarker:  scanMarker,
	}
}

func PathFromEnv(envKey string, defaultFileName string) string {
	path := strings.TrimSpace(os.Getenv(envKey))
	if path != "" {
		return path
	}
	return filepath.Join(os.TempDir(), defaultFileName)
}

func (c Controller) StartBackground(subcommand string, subcommandArgs []string, logPath string) (int, error) {
	executablePath, err := os.Executable()
	if err != nil {
		return 0, fmt.Errorf("resolve executable path: %w", err)
	}

	commandArgs := append([]string{subcommand}, subcommandArgs...)
	return c.StartBackgroundCommand(executablePath, commandArgs, logPath)
}

func (c Controller) StartBackgroundCommand(executablePath string, commandArgs []string, logPath string) (int, error) {
	if err := os.MkdirAll(filepath.Dir(logPath), 0o755); err != nil {
		return 0, fmt.Errorf("create log directory: %w", err)
	}

	logFile, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return 0, fmt.Errorf("open log file: %w", err)
	}
	defer logFile.Close()

	command := exec.Command(executablePath, commandArgs...)
	command.Stdout = logFile
	command.Stderr = logFile

	if err := command.Start(); err != nil {
		return 0, err
	}

	pid := command.Process.Pid
	if err := c.WritePID(pid); err != nil {
		_ = command.Process.Kill()
		return 0, fmt.Errorf("write pid file: %w", err)
	}

	time.Sleep(startProbeDelay)
	if !IsProcessRunning(pid) {
		_ = c.RemovePIDFile()
		return 0, fmt.Errorf("process failed to stay running")
	}

	return pid, nil
}

func (c Controller) Stop(waitAttempts int, waitDelay time.Duration) (int, bool, error) {
	pid, ok := c.resolveRunningPID(false)
	if !ok {
		_ = c.RemovePIDFile()
		return 0, false, nil
	}

	process, err := os.FindProcess(pid)
	if err != nil {
		return pid, true, fmt.Errorf("find process: %w", err)
	}

	if err := process.Signal(os.Interrupt); err != nil {
		if killErr := process.Kill(); killErr != nil {
			return pid, true, fmt.Errorf("stop process: %w", killErr)
		}
	}

	if waitAttempts <= 0 {
		waitAttempts = defaultStopWaitAttempts
	}
	if waitDelay <= 0 {
		waitDelay = defaultStopWaitDelay
	}

	for range waitAttempts {
		if !IsProcessRunning(pid) {
			_ = c.RemovePIDFile()
			return pid, true, nil
		}
		time.Sleep(waitDelay)
	}

	if err := process.Kill(); err != nil {
		return pid, true, fmt.Errorf("kill process: %w", err)
	}

	_ = c.RemovePIDFile()
	return pid, true, nil
}

func (c Controller) LoadRunningPID() (int, bool) {
	return c.resolveRunningPID(true)
}

func (c Controller) resolveRunningPID(syncFallbackPID bool) (int, bool) {
	pid, err := c.ReadPID()
	if err == nil && IsProcessRunning(pid) {
		return pid, true
	}

	fallbackPID, ok := c.findRunningPIDByScan()
	if !ok {
		_ = c.RemovePIDFile()
		return 0, false
	}

	if syncFallbackPID {
		_ = c.WritePID(fallbackPID)
	}
	return fallbackPID, true
}

func (c Controller) WritePID(pid int) error {
	if err := os.MkdirAll(filepath.Dir(c.PIDFilePath), 0o755); err != nil {
		return err
	}
	return os.WriteFile(c.PIDFilePath, []byte(strconv.Itoa(pid)), 0o644)
}

func (c Controller) ReadPID() (int, error) {
	data, err := os.ReadFile(c.PIDFilePath)
	if err != nil {
		return 0, err
	}

	value := strings.TrimSpace(string(data))
	if value == "" {
		return 0, fmt.Errorf("pid file is empty")
	}

	pid, err := strconv.Atoi(value)
	if err != nil {
		return 0, err
	}
	if pid <= 0 {
		return 0, fmt.Errorf("invalid pid")
	}

	return pid, nil
}

func (c Controller) RemovePIDFile() error {
	err := os.Remove(c.PIDFilePath)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

func IsProcessRunning(pid int) bool {
	if pid <= 0 {
		return false
	}

	process, err := os.FindProcess(pid)
	if err != nil {
		return false
	}

	err = process.Signal(syscall.Signal(0))
	if err == nil {
		return !isZombieProcess(pid)
	}

	message := strings.ToLower(err.Error())
	if strings.Contains(message, "process already finished") || strings.Contains(message, "no such process") {
		return false
	}

	return true
}

func isZombieProcess(pid int) bool {
	output, err := exec.Command("ps", "-o", "stat=", "-p", strconv.Itoa(pid)).Output()
	if err != nil {
		return false
	}

	state := strings.TrimSpace(string(output))
	if state == "" {
		return false
	}

	return strings.HasPrefix(state, "Z") || strings.Contains(state, " Z")
}

func (c Controller) findRunningPIDByScan() (int, bool) {
	if c.ScanMarker == "" {
		return 0, false
	}

	output, err := exec.Command("pgrep", "-f", c.ScanMarker).Output()
	if err != nil {
		return 0, false
	}

	lines := strings.SplitSeq(strings.TrimSpace(string(output)), "\n")
	for line := range lines {
		value := strings.TrimSpace(line)
		if value == "" {
			continue
		}

		pid, parseErr := strconv.Atoi(value)
		if parseErr != nil {
			continue
		}
		if pid <= 0 || pid == os.Getpid() {
			continue
		}
		if !IsProcessRunning(pid) {
			continue
		}

		commandOutput, commandErr := exec.Command("ps", "-o", "command=", "-p", strconv.Itoa(pid)).Output()
		if commandErr != nil {
			continue
		}

		commandLine := strings.TrimSpace(string(commandOutput))
		if commandLine == "" {
			continue
		}
		if strings.Contains(commandLine, c.ScanMarker) {
			return pid, true
		}
	}

	return 0, false
}
