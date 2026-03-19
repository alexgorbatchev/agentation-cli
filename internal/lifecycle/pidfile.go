package lifecycle

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

func pidFilePath() string {
	path := strings.TrimSpace(os.Getenv("AGENTATION_PID_FILE"))
	if path != "" {
		return path
	}
	return filepath.Join(os.TempDir(), "agentation.pid")
}

func writePID(pid int) error {
	path := pidFilePath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(strconv.Itoa(pid)), 0o644)
}

func readPID() (int, error) {
	data, err := os.ReadFile(pidFilePath())
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

func removePIDFile() error {
	err := os.Remove(pidFilePath())
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

func isProcessRunning(pid int) bool {
	if pid <= 0 {
		return false
	}

	process, err := os.FindProcess(pid)
	if err != nil {
		return false
	}

	err = process.Signal(syscall.Signal(0))
	if err == nil {
		return true
	}

	message := strings.ToLower(err.Error())
	if strings.Contains(message, "process already finished") || strings.Contains(message, "no such process") {
		return false
	}

	return true
}

func loadRunningPID() (int, bool) {
	pid, err := readPID()
	if err == nil && isProcessRunning(pid) {
		return pid, true
	}

	fallbackPID, ok := findRunningPIDByScan()
	if !ok {
		_ = removePIDFile()
		return 0, false
	}

	_ = writePID(fallbackPID)
	return fallbackPID, true
}

func findRunningPIDByScan() (int, bool) {
	output, err := exec.Command("pgrep", "-f", "__serve-stack").Output()
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
		if !isProcessRunning(pid) {
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

		if strings.Contains(commandLine, "__serve-stack") {
			return pid, true
		}
	}

	return 0, false
}
