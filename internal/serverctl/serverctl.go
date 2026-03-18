package serverctl

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/benjitaylor/agentation/cli/internal/server"
)

const shutdownTimeout = 5 * time.Second

type serveConfig struct {
	address string
}

func Run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		printUsage(stdout)
		return 0
	}

	subcommand := args[0]
	subArgs := args[1:]

	switch subcommand {
	case "serve":
		return runServe(subArgs, stdout, stderr)
	case "start":
		return runStart(subArgs, stdout, stderr)
	case "stop":
		return runStop(stdout, stderr)
	case "status":
		return runStatus(stdout)
	case "help", "--help", "-h":
		printUsage(stdout)
		return 0
	default:
		return runServe(args, stdout, stderr)
	}
}

func runServe(args []string, stdout, stderr io.Writer) int {
	cfg, err := parseServeFlags(args, stderr)
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		fmt.Fprintf(stderr, "failed to parse serve flags: %v\n", err)
		return 1
	}

	logger := slog.New(slog.NewTextHandler(stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	service := server.NewService(cfg.address, logger)

	go func() {
		err := service.ListenAndServe()
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("agentation server failed", "error", err)
			os.Exit(1)
		}
	}()

	signalContext, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	<-signalContext.Done()

	shutdownContext, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()

	if err := service.Shutdown(shutdownContext); err != nil {
		logger.Error("agentation server shutdown failed", "error", err)
		return 1
	}

	logger.Info("agentation server stopped")
	return 0
}

func runStart(args []string, stdout, stderr io.Writer) int {
	foreground, serveArgs := parseStartArgs(args)

	if pid, ok := loadRunningPID(); ok {
		fmt.Fprintf(stdout, "agentation server already running (pid %d)\n", pid)
		return 0
	}

	if foreground {
		fmt.Fprintln(stdout, "starting agentation server in foreground")
		return runServe(serveArgs, stdout, stderr)
	}

	executablePath, err := os.Executable()
	if err != nil {
		fmt.Fprintf(stderr, "failed to resolve executable path: %v\n", err)
		return 1
	}

	logPath := logFilePath()
	if err := os.MkdirAll(filepath.Dir(logPath), 0o755); err != nil {
		fmt.Fprintf(stderr, "failed to create log directory: %v\n", err)
		return 1
	}

	logFile, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		fmt.Fprintf(stderr, "failed to open log file: %v\n", err)
		return 1
	}
	defer logFile.Close()

	commandArgs := append([]string{"__serve-server"}, serveArgs...)
	command := exec.Command(executablePath, commandArgs...)
	command.Stdout = logFile
	command.Stderr = logFile

	if err := command.Start(); err != nil {
		fmt.Fprintf(stderr, "failed to start agentation server: %v\n", err)
		return 1
	}

	pid := command.Process.Pid
	if err := writePID(pid); err != nil {
		fmt.Fprintf(stderr, "failed to write pid file: %v\n", err)
		_ = command.Process.Kill()
		return 1
	}

	time.Sleep(250 * time.Millisecond)
	if !isProcessRunning(pid) {
		_ = removePIDFile()
		fmt.Fprintln(stderr, "agentation server failed to stay running")
		return 1
	}

	fmt.Fprintf(stdout, "agentation server started in background (pid %d)\n", pid)
	fmt.Fprintf(stdout, "log: %s\n", logPath)
	return 0
}

func runStop(stdout, stderr io.Writer) int {
	pid, err := readPID()
	if err != nil || !isProcessRunning(pid) {
		fallbackPID, ok := findRunningServerPIDByScan()
		if !ok {
			_ = removePIDFile()
			fmt.Fprintln(stdout, "agentation server is not running")
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
			fmt.Fprintf(stderr, "failed to stop agentation server: %v\n", killErr)
			return 1
		}
	}

	for attempt := 0; attempt < 30; attempt++ {
		if !isProcessRunning(pid) {
			_ = removePIDFile()
			fmt.Fprintf(stdout, "agentation server stopped (pid %d)\n", pid)
			return 0
		}
		time.Sleep(100 * time.Millisecond)
	}

	if err := process.Kill(); err != nil {
		fmt.Fprintf(stderr, "failed to kill agentation server: %v\n", err)
		return 1
	}

	_ = removePIDFile()
	fmt.Fprintf(stdout, "agentation server stopped (pid %d)\n", pid)
	return 0
}

func runStatus(stdout io.Writer) int {
	pid, err := readPID()
	if err != nil || !isProcessRunning(pid) {
		fallbackPID, ok := findRunningServerPIDByScan()
		if !ok {
			_ = removePIDFile()
			fmt.Fprintln(stdout, "agentation server not running")
			return 1
		}
		pid = fallbackPID
		_ = writePID(pid)
	}

	fmt.Fprintf(stdout, "agentation server running (pid %d)\n", pid)
	return 0
}

func parseServeFlags(args []string, stderr io.Writer) (serveConfig, error) {
	cfg := serveConfig{address: "127.0.0.1:4747"}

	flags := flag.NewFlagSet("agentation server serve", flag.ContinueOnError)
	flags.SetOutput(stderr)
	flags.StringVar(&cfg.address, "address", cfg.address, "HTTP listen address")

	if err := flags.Parse(args); err != nil {
		return serveConfig{}, err
	}

	cfg.address = strings.TrimSpace(cfg.address)
	if cfg.address == "" {
		cfg.address = "127.0.0.1:4747"
	}

	return cfg, nil
}

func parseStartArgs(args []string) (bool, []string) {
	foreground := false
	serveArgs := make([]string, 0, len(args))
	for _, arg := range args {
		switch arg {
		case "--foreground", "foreground":
			foreground = true
		case "--background", "background":
			foreground = false
		default:
			serveArgs = append(serveArgs, arg)
		}
	}
	return foreground, serveArgs
}

func pidFilePath() string {
	path := strings.TrimSpace(os.Getenv("AGENTATION_SERVER_PID_FILE"))
	if path != "" {
		return path
	}
	return filepath.Join(os.TempDir(), "agentation-server.pid")
}

func logFilePath() string {
	path := strings.TrimSpace(os.Getenv("AGENTATION_SERVER_LOG_FILE"))
	if path != "" {
		return path
	}
	return filepath.Join(os.TempDir(), "agentation-server.log")
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

	fallbackPID, ok := findRunningServerPIDByScan()
	if !ok {
		_ = removePIDFile()
		return 0, false
	}

	_ = writePID(fallbackPID)
	return fallbackPID, true
}

func findRunningServerPIDByScan() (int, bool) {
	output, err := exec.Command("pgrep", "-f", "__serve-server").Output()
	if err != nil {
		return 0, false
	}

	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	for _, line := range lines {
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

		cmdOutput, cmdErr := exec.Command("ps", "-o", "command=", "-p", strconv.Itoa(pid)).Output()
		if cmdErr != nil {
			continue
		}

		commandLine := strings.TrimSpace(string(cmdOutput))
		if commandLine == "" {
			continue
		}

		if strings.Contains(commandLine, "__serve-server") {
			return pid, true
		}
	}

	return 0, false
}

func printUsage(writer io.Writer) {
	fmt.Fprintln(writer, "agentation server commands:")
	fmt.Fprintln(writer, "  serve [--address 127.0.0.1:4747]")
	fmt.Fprintln(writer, "  start [--foreground|--background] [serve flags]")
	fmt.Fprintln(writer, "  stop")
	fmt.Fprintln(writer, "  status")
}
