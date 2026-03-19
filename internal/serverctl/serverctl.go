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
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/benjitaylor/agentation/cli/internal/procctl"
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
	controller := serverController()

	if pid, ok := controller.LoadRunningPID(); ok {
		fmt.Fprintf(stdout, "agentation server already running (pid %d)\n", pid)
		return 0
	}

	if foreground {
		fmt.Fprintln(stdout, "starting agentation server in foreground")
		return runServe(serveArgs, stdout, stderr)
	}

	logPath := logFilePath()
	pid, err := controller.StartBackground("__serve-server", serveArgs, logPath)
	if err != nil {
		fmt.Fprintf(stderr, "failed to start agentation server: %v\n", err)
		return 1
	}

	fmt.Fprintf(stdout, "agentation server started in background (pid %d)\n", pid)
	fmt.Fprintf(stdout, "log: %s\n", logPath)
	return 0
}

func runStop(stdout, stderr io.Writer) int {
	pid, stopped, err := serverController().Stop(30, 100*time.Millisecond)
	if err != nil {
		fmt.Fprintf(stderr, "failed to stop agentation server: %v\n", err)
		return 1
	}
	if !stopped {
		fmt.Fprintln(stdout, "agentation server is not running")
		return 0
	}

	fmt.Fprintf(stdout, "agentation server stopped (pid %d)\n", pid)
	return 0
}

func runStatus(stdout io.Writer) int {
	pid, ok := serverController().LoadRunningPID()
	if !ok {
		fmt.Fprintln(stdout, "agentation server not running")
		return 1
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
	return procctl.PathFromEnv("AGENTATION_SERVER_PID_FILE", "agentation-server.pid")
}

func logFilePath() string {
	return procctl.PathFromEnv("AGENTATION_SERVER_LOG_FILE", "agentation-server.log")
}

func serverController() procctl.Controller {
	return procctl.New(pidFilePath(), "__serve-server")
}

func printUsage(writer io.Writer) {
	fmt.Fprintln(writer, "agentation server commands:")
	fmt.Fprintln(writer, "  serve [--address 127.0.0.1:4747]")
	fmt.Fprintln(writer, "  start [--foreground|--background] [serve flags]")
	fmt.Fprintln(writer, "  stop")
	fmt.Fprintln(writer, "  status")
}
