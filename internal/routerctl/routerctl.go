package routerctl

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
	"syscall"
	"time"

	"github.com/alexgorbatchev/agentation-cli/internal/procctl"
	"github.com/alexgorbatchev/agentation-cli/internal/router/config"
	httpserver "github.com/alexgorbatchev/agentation-cli/internal/router/http"
	routerpkg "github.com/alexgorbatchev/agentation-cli/internal/router/router"
	"github.com/alexgorbatchev/agentation-cli/internal/router/store"
)

const shutdownTimeout = 5 * time.Second

func Run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		if err := printUsage(stdout); err != nil {
			return 1
		}
		return 0
	}

	subcommand := args[0]
	subcommandArgs := args[1:]

	switch subcommand {
	case "serve":
		return runServe(subcommandArgs, stdout, stderr)
	case "start":
		return runStart(subcommandArgs, stdout, stderr)
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
	cfg, err := config.Load(args, stderr)
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		if writeErr := writef(stderr, "failed to parse router serve flags: %v\n", err); writeErr != nil {
			return 1
		}
		return 1
	}

	logger := slog.New(slog.NewTextHandler(stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

	registry := store.NewRegistry(cfg.SessionStaleAfter)
	forwarder := routerpkg.NewForwarder(cfg.ForwardTimeout)
	server := httpserver.NewServer(cfg, logger, registry, forwarder)

	go func() {
		logger.Info("agentation router listening", "address", cfg.Address)
		err := server.ListenAndServe()
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("router server failed", "error", err)
			os.Exit(1)
		}
	}()

	signalContext, stopSignal := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stopSignal()

	<-signalContext.Done()
	shutdownContext, cancelShutdown := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancelShutdown()

	if err := server.Shutdown(shutdownContext); err != nil {
		logger.Error("router shutdown failed", "error", err)
		return 1
	}

	logger.Info("agentation router stopped")
	return 0
}

func runStart(args []string, stdout, stderr io.Writer) int {
	foreground, serveArgs := parseStartArgs(args)
	controller := routerController()

	if pid, ok := controller.LoadRunningPID(); ok {
		if err := writef(stdout, "agentation router already running (pid %d)\n", pid); err != nil {
			return 1
		}
		return 0
	}

	if foreground {
		if err := writeln(stdout, "starting agentation router in foreground"); err != nil {
			return 1
		}
		return runServe(serveArgs, stdout, stderr)
	}

	logPath := logFilePath()
	pid, err := controller.StartBackground("__serve-router", serveArgs, logPath)
	if err != nil {
		if writeErr := writef(stderr, "failed to start agentation router: %v\n", err); writeErr != nil {
			return 1
		}
		return 1
	}

	if err := writef(stdout, "agentation router started in background (pid %d)\n", pid); err != nil {
		return 1
	}
	if err := writef(stdout, "log: %s\n", logPath); err != nil {
		return 1
	}
	return 0
}

func runStop(stdout, stderr io.Writer) int {
	pid, stopped, err := routerController().Stop(30, 100*time.Millisecond)
	if err != nil {
		if writeErr := writef(stderr, "failed to stop agentation router: %v\n", err); writeErr != nil {
			return 1
		}
		return 1
	}
	if !stopped {
		if err := writeln(stdout, "agentation router is not running"); err != nil {
			return 1
		}
		return 0
	}

	if err := writef(stdout, "agentation router stopped (pid %d)\n", pid); err != nil {
		return 1
	}
	return 0
}

func runStatus(stdout io.Writer) int {
	pid, ok := routerController().LoadRunningPID()
	if !ok {
		if err := writeln(stdout, "agentation router not running"); err != nil {
			return 1
		}
		return 1
	}

	if err := writef(stdout, "agentation router running (pid %d)\n", pid); err != nil {
		return 1
	}
	return 0
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
	return procctl.PathFromEnv("AGENTATION_ROUTER_PID_FILE", "agentation-router.pid")
}

func logFilePath() string {
	return procctl.PathFromEnv("AGENTATION_ROUTER_LOG_FILE", "agentation-router.log")
}

func routerController() procctl.Controller {
	return procctl.New(pidFilePath(), "__serve-router")
}

func printUsage(writer io.Writer) error {
	if err := writeln(writer, "agentation router internal commands:"); err != nil {
		return err
	}
	if err := writeln(writer, "  start [--foreground|--background] [serve flags]"); err != nil {
		return err
	}
	if err := writeln(writer, "  stop"); err != nil {
		return err
	}
	if err := writeln(writer, "  status"); err != nil {
		return err
	}
	return writeln(writer, "  serve [flags] (run foreground server)")
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
