package lifecycle

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"time"
)

func runStopCommand(args []string, stdout, stderr io.Writer) int {
	if err := parseNoArgCommand("stop", args, stderr); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		fmt.Fprintf(stderr, "failed to parse stop flags: %v\n", err)
		return 1
	}

	pid, err := readPID()
	if err != nil || !isProcessRunning(pid) {
		fallbackPID, ok := findRunningPIDByScan()
		if !ok {
			_ = removePIDFile()
			fmt.Fprintln(stdout, "agentation is not running")
			return 0
		}
		pid = fallbackPID
	}

	process, err := os.FindProcess(pid)
	if err != nil {
		fmt.Fprintf(stderr, "failed to find process: %v\n", err)
		return 1
	}

	if err := process.Signal(os.Interrupt); err != nil {
		if killErr := process.Kill(); killErr != nil {
			fmt.Fprintf(stderr, "failed to stop agentation: %v\n", killErr)
			return 1
		}
	}

	for range 30 {
		if !isProcessRunning(pid) {
			_ = removePIDFile()
			fmt.Fprintf(stdout, "agentation stopped (pid %d)\n", pid)
			return 0
		}
		time.Sleep(100 * time.Millisecond)
	}

	if err := process.Kill(); err != nil {
		fmt.Fprintf(stderr, "failed to kill agentation: %v\n", err)
		return 1
	}

	_ = removePIDFile()
	fmt.Fprintf(stdout, "agentation stopped (pid %d)\n", pid)
	return 0
}

func runStatusCommand(args []string, stdout, stderr io.Writer) int {
	if err := parseNoArgCommand("status", args, stderr); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		fmt.Fprintf(stderr, "failed to parse status flags: %v\n", err)
		return 1
	}

	pid, err := readPID()
	if err != nil || !isProcessRunning(pid) {
		fallbackPID, ok := findRunningPIDByScan()
		if !ok {
			_ = removePIDFile()
			fmt.Fprintln(stdout, "agentation not running")
			return 1
		}
		pid = fallbackPID
		_ = writePID(pid)
	}

	fmt.Fprintf(stdout, "agentation running (pid %d)\n", pid)
	return 0
}
