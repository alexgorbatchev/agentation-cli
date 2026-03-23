package lifecycle

import (
	"errors"
	"flag"
	"io"
	"os"
	"time"
)

func runStopCommand(args []string, stdout, stderr io.Writer) int {
	if err := parseNoArgCommand("stop", args, stderr); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		if writeErr := writef(stderr, "failed to parse stop flags: %v\n", err); writeErr != nil {
			return 1
		}
		return 1
	}

	pid, err := readPID()
	if err != nil || !isProcessRunning(pid) {
		fallbackPID, ok := findRunningPIDByScan()
		if !ok {
			_ = removePIDFile()
			if err := writeln(stdout, "agentation is not running"); err != nil {
				return 1
			}
			return 0
		}
		pid = fallbackPID
	}

	process, err := os.FindProcess(pid)
	if err != nil {
		if writeErr := writef(stderr, "failed to find process: %v\n", err); writeErr != nil {
			return 1
		}
		return 1
	}

	if err := process.Signal(os.Interrupt); err != nil {
		if killErr := process.Kill(); killErr != nil {
			if writeErr := writef(stderr, "failed to stop agentation: %v\n", killErr); writeErr != nil {
				return 1
			}
			return 1
		}
	}

	for range 30 {
		if !isProcessRunning(pid) {
			_ = removePIDFile()
			if err := writef(stdout, "agentation stopped (pid %d)\n", pid); err != nil {
				return 1
			}
			return 0
		}
		time.Sleep(100 * time.Millisecond)
	}

	if err := process.Kill(); err != nil {
		if writeErr := writef(stderr, "failed to kill agentation: %v\n", err); writeErr != nil {
			return 1
		}
		return 1
	}

	_ = removePIDFile()
	if err := writef(stdout, "agentation stopped (pid %d)\n", pid); err != nil {
		return 1
	}
	return 0
}

func runStatusCommand(args []string, stdout, stderr io.Writer) int {
	if err := parseNoArgCommand("status", args, stderr); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		if writeErr := writef(stderr, "failed to parse status flags: %v\n", err); writeErr != nil {
			return 1
		}
		return 1
	}

	pid, err := readPID()
	if err != nil || !isProcessRunning(pid) {
		fallbackPID, ok := findRunningPIDByScan()
		if !ok {
			_ = removePIDFile()
			if err := writeln(stdout, "agentation not running"); err != nil {
				return 1
			}
			return 1
		}
		pid = fallbackPID
		_ = writePID(pid)
	}

	if err := writef(stdout, "agentation running (pid %d)\n", pid); err != nil {
		return 1
	}
	return 0
}
