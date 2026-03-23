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

	"github.com/alexgorbatchev/agentation-cli/internal/procctl"
	"github.com/alexgorbatchev/agentation-cli/internal/server"
)

const shutdownTimeout = 5 * time.Second

type serveConfig struct {
	address string
}

func Run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		if err := printUsage(stdout); err != nil {
			return 1
		}
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
		if err := printUsage(stdout); err != nil {
			return 1
		}
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
		if writeErr := writef(stderr, "failed to parse serve flags: %v\n", err); writeErr != nil {
			return 1
		}
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
		if err := writef(stdout, "agentation server already running (pid %d)\n", pid); err != nil {
			return 1
		}
		return 0
	}

	if foreground {
		if err := writeln(stdout, "starting agentation server in foreground"); err != nil {
			return 1
		}
		return runServe(serveArgs, stdout, stderr)
	}

	logPath := logFilePath()
	pid, err := controller.StartBackground("__serve-server", serveArgs, logPath)
	if err != nil {
		if writeErr := writef(stderr, "failed to start agentation server: %v\n", err); writeErr != nil {
			return 1
		}
		return 1
	}

	if err := writef(stdout, "agentation server started in background (pid %d)\n", pid); err != nil {
		return 1
	}
	if err := writef(stdout, "log: %s\n", logPath); err != nil {
		return 1
	}
	return 0
}

func runStop(stdout, stderr io.Writer) int {
	pid, stopped, err := serverController().Stop(30, 100*time.Millisecond)
	if err != nil {
		if writeErr := writef(stderr, "failed to stop agentation server: %v\n", err); writeErr != nil {
			return 1
		}
		return 1
	}
	if !stopped {
		if err := writeln(stdout, "agentation server is not running"); err != nil {
			return 1
		}
		return 0
	}

	if err := writef(stdout, "agentation server stopped (pid %d)\n", pid); err != nil {
		return 1
	}
	return 0
}

func runStatus(stdout io.Writer) int {
	pid, ok := serverController().LoadRunningPID()
	if !ok {
		if err := writeln(stdout, "agentation server not running"); err != nil {
			return 1
		}
		return 1
	}

	if err := writef(stdout, "agentation server running (pid %d)\n", pid); err != nil {
		return 1
	}
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

func printUsage(writer io.Writer) error {
	if err := writeln(writer, "agentation server commands:"); err != nil {
		return err
	}
	if err := writeln(writer, "  serve [--address 127.0.0.1:4747]"); err != nil {
		return err
	}
	if err := writeln(writer, "  start [--foreground|--background] [serve flags]"); err != nil {
		return err
	}
	if err := writeln(writer, "  stop"); err != nil {
		return err
	}
	return writeln(writer, "  status")
}

func writef(writer io.Writer, format string, args ...any) error {
	if _, err := fmt.Fprintf(writer, format, args...); err != nil {
		return fmt.Errorf("writing output: %w", err)
	}
	return nil
}

func writeln(writer io.Writer, args ...any) error {
	if _, err := fmt.Fprintln(writer, args...); err != nil {
		return fmt.Errorf("writing output: %w", err)
	}
	return nil
}
