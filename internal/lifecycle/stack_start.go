package lifecycle

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
	"syscall"
	"time"

	routerconfig "github.com/alexgorbatchev/agentation-cli/internal/router/config"
	routerhttp "github.com/alexgorbatchev/agentation-cli/internal/router/http"
	routerpkg "github.com/alexgorbatchev/agentation-cli/internal/router/router"
	routerstore "github.com/alexgorbatchev/agentation-cli/internal/router/store"
	"github.com/alexgorbatchev/agentation-cli/internal/server"
)

func runStartCommand(args []string, stdout, stderr io.Writer) int {
	cfg, err := parseStartFlags(args, stderr)
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		if writeErr := writef(stderr, "failed to parse start flags: %v\n", err); writeErr != nil {
			return 1
		}
		return 1
	}

	if pid, ok := loadRunningPID(); ok {
		if err := writef(stdout, "agentation already running (pid %d)\n", pid); err != nil {
			return 1
		}
		return 0
	}

	if cfg.foreground {
		return runServeStack(cfg.serve, stdout, stderr)
	}

	executablePath, err := os.Executable()
	if err != nil {
		if writeErr := writef(stderr, "failed to resolve executable path: %v\n", err); writeErr != nil {
			return 1
		}
		return 1
	}

	stackLogPath := stackLogFilePath()
	if err := os.MkdirAll(filepath.Dir(stackLogPath), 0o755); err != nil {
		if writeErr := writef(stderr, "failed to create log directory: %v\n", err); writeErr != nil {
			return 1
		}
		return 1
	}

	stackLogFile, err := os.OpenFile(stackLogPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		if writeErr := writef(stderr, "failed to open log file: %v\n", err); writeErr != nil {
			return 1
		}
		return 1
	}
	defer reportCloseError(stderr, "stack log file", stackLogFile)

	commandArgs := []string{
		"__serve-stack",
		"--server-addr", cfg.serve.serverAddr,
		"--router-addr", cfg.serve.routerAddr,
	}
	command := exec.Command(executablePath, commandArgs...)
	command.Stdout = stackLogFile
	command.Stderr = stackLogFile

	if err := command.Start(); err != nil {
		if writeErr := writef(stderr, "failed to start agentation: %v\n", err); writeErr != nil {
			return 1
		}
		return 1
	}

	pid := command.Process.Pid
	if err := writePID(pid); err != nil {
		if writeErr := writef(stderr, "failed to write pid file: %v\n", err); writeErr != nil {
			return 1
		}
		_ = command.Process.Kill()
		return 1
	}

	time.Sleep(250 * time.Millisecond)
	if !isProcessRunning(pid) {
		_ = removePIDFile()
		if err := writeln(stderr, "agentation failed to stay running"); err != nil {
			return 1
		}
		return 1
	}

	if err := writef(stdout, "agentation started in background (pid %d)\n", pid); err != nil {
		return 1
	}
	if err := writef(stdout, "log: %s\n", stackLogPath); err != nil {
		return 1
	}
	if cfg.serve.enableServer {
		if err := writef(stdout, "server log: %s\n", serverLogFilePath()); err != nil {
			return 1
		}
	}
	if cfg.serve.enableRouter {
		if err := writef(stdout, "router log: %s\n", routerLogFilePath()); err != nil {
			return 1
		}
	}

	return 0
}

func runServeCommand(args []string, stdout, stderr io.Writer) int {
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

	return runServeStack(cfg, stdout, stderr)
}

func runServeStack(cfg serveConfig, stdout, stderr io.Writer) int {
	if !cfg.enableServer && !cfg.enableRouter {
		if err := writeln(stderr, "nothing to serve: both server and router are disabled"); err != nil {
			return 1
		}
		return 1
	}

	serverWriter, serverCloser, err := openServiceLogWriter(serverLogFilePath(), stdout)
	if err != nil {
		if writeErr := writef(stderr, "failed to open server log file: %v\n", err); writeErr != nil {
			return 1
		}
		return 1
	}
	if serverCloser != nil {
		defer reportCloseError(stderr, "server log file", serverCloser)
	}

	routerWriter, routerCloser, err := openServiceLogWriter(routerLogFilePath(), stdout)
	if err != nil {
		if writeErr := writef(stderr, "failed to open router log file: %v\n", err); writeErr != nil {
			return 1
		}
		return 1
	}
	if routerCloser != nil {
		defer reportCloseError(stderr, "router log file", routerCloser)
	}

	signalContext, stopSignal := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stopSignal()

	serveErrors := make(chan error, 2)

	var serverService *server.Service
	if cfg.enableServer {
		serverLogger := slog.New(slog.NewTextHandler(serverWriter, &slog.HandlerOptions{Level: slog.LevelInfo}))
		serverService = server.NewService(cfg.serverAddr, serverLogger)

		go func() {
			err := serverService.ListenAndServe()
			if err != nil && !errors.Is(err, http.ErrServerClosed) {
				serveErrors <- fmt.Errorf("server failed: %w", err)
			}
		}()
	} else {
		if err := writeln(stdout, "agentation server disabled"); err != nil {
			return 1
		}
	}

	var routerService *http.Server
	if cfg.enableRouter {
		routerCfg, cfgErr := routerconfig.Load([]string{"--address", cfg.routerAddr}, stderr)
		if cfgErr != nil {
			if writeErr := writef(stderr, "failed to build router config: %v\n", cfgErr); writeErr != nil {
				return 1
			}
			return 1
		}

		routerLogger := slog.New(slog.NewTextHandler(routerWriter, &slog.HandlerOptions{Level: slog.LevelInfo}))
		registry := routerstore.NewRegistry(routerCfg.SessionStaleAfter)
		forwarder := routerpkg.NewForwarder(routerCfg.ForwardTimeout)
		routerService = routerhttp.NewServer(routerCfg, routerLogger, registry, forwarder)

		go func() {
			err := routerService.ListenAndServe()
			if err != nil && !errors.Is(err, http.ErrServerClosed) {
				serveErrors <- fmt.Errorf("router failed: %w", err)
			}
		}()
	} else {
		if err := writeln(stdout, "agentation router disabled"); err != nil {
			return 1
		}
	}

	select {
	case <-signalContext.Done():
	case err := <-serveErrors:
		if writeErr := writef(stderr, "%v\n", err); writeErr != nil {
			return 1
		}
		return 1
	}

	shutdownContext, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()

	shutdownErr := false
	if routerService != nil {
		if err := routerService.Shutdown(shutdownContext); err != nil && !errors.Is(err, context.Canceled) {
			if writeErr := writef(stderr, "agentation router shutdown failed: %v\n", err); writeErr != nil {
				return 1
			}
			shutdownErr = true
		}
	}

	if serverService != nil {
		if err := serverService.Shutdown(shutdownContext); err != nil && !errors.Is(err, context.Canceled) {
			if writeErr := writef(stderr, "agentation server shutdown failed: %v\n", err); writeErr != nil {
				return 1
			}
			shutdownErr = true
		}
	}

	if shutdownErr {
		return 1
	}

	return 0
}
