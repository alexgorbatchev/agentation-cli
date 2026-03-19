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

	routerconfig "github.com/benjitaylor/agentation/cli/internal/router/config"
	routerhttp "github.com/benjitaylor/agentation/cli/internal/router/http"
	routerpkg "github.com/benjitaylor/agentation/cli/internal/router/router"
	routerstore "github.com/benjitaylor/agentation/cli/internal/router/store"
	"github.com/benjitaylor/agentation/cli/internal/server"
)

func runStartCommand(args []string, stdout, stderr io.Writer) int {
	cfg, err := parseStartFlags(args, stderr)
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		fmt.Fprintf(stderr, "failed to parse start flags: %v\n", err)
		return 1
	}

	if pid, ok := loadRunningPID(); ok {
		fmt.Fprintf(stdout, "agentation already running (pid %d)\n", pid)
		return 0
	}

	if cfg.foreground {
		return runServeStack(cfg.serve, stdout, stderr)
	}

	executablePath, err := os.Executable()
	if err != nil {
		fmt.Fprintf(stderr, "failed to resolve executable path: %v\n", err)
		return 1
	}

	stackLogPath := stackLogFilePath()
	if err := os.MkdirAll(filepath.Dir(stackLogPath), 0o755); err != nil {
		fmt.Fprintf(stderr, "failed to create log directory: %v\n", err)
		return 1
	}

	stackLogFile, err := os.OpenFile(stackLogPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		fmt.Fprintf(stderr, "failed to open log file: %v\n", err)
		return 1
	}
	defer stackLogFile.Close()

	commandArgs := []string{
		"__serve-stack",
		"--server-addr", cfg.serve.serverAddr,
		"--router-addr", cfg.serve.routerAddr,
	}
	command := exec.Command(executablePath, commandArgs...)
	command.Stdout = stackLogFile
	command.Stderr = stackLogFile

	if err := command.Start(); err != nil {
		fmt.Fprintf(stderr, "failed to start agentation: %v\n", err)
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
		fmt.Fprintln(stderr, "agentation failed to stay running")
		return 1
	}

	fmt.Fprintf(stdout, "agentation started in background (pid %d)\n", pid)
	fmt.Fprintf(stdout, "log: %s\n", stackLogPath)
	if cfg.serve.enableServer {
		fmt.Fprintf(stdout, "server log: %s\n", serverLogFilePath())
	}
	if cfg.serve.enableRouter {
		fmt.Fprintf(stdout, "router log: %s\n", routerLogFilePath())
	}

	return 0
}

func runServeCommand(args []string, stdout, stderr io.Writer) int {
	cfg, err := parseServeFlags(args, stderr)
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		fmt.Fprintf(stderr, "failed to parse serve flags: %v\n", err)
		return 1
	}

	return runServeStack(cfg, stdout, stderr)
}

func runServeStack(cfg serveConfig, stdout, stderr io.Writer) int {
	if !cfg.enableServer && !cfg.enableRouter {
		fmt.Fprintln(stderr, "nothing to serve: both server and router are disabled")
		return 1
	}

	serverWriter, serverCloser, err := openServiceLogWriter(serverLogFilePath(), stdout)
	if err != nil {
		fmt.Fprintf(stderr, "failed to open server log file: %v\n", err)
		return 1
	}
	if serverCloser != nil {
		defer serverCloser.Close()
	}

	routerWriter, routerCloser, err := openServiceLogWriter(routerLogFilePath(), stdout)
	if err != nil {
		fmt.Fprintf(stderr, "failed to open router log file: %v\n", err)
		return 1
	}
	if routerCloser != nil {
		defer routerCloser.Close()
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
		fmt.Fprintln(stdout, "agentation server disabled")
	}

	var routerService *http.Server
	if cfg.enableRouter {
		routerCfg, cfgErr := routerconfig.Load([]string{"--address", cfg.routerAddr}, stderr)
		if cfgErr != nil {
			fmt.Fprintf(stderr, "failed to build router config: %v\n", cfgErr)
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
		fmt.Fprintln(stdout, "agentation router disabled")
	}

	select {
	case <-signalContext.Done():
	case err := <-serveErrors:
		fmt.Fprintf(stderr, "%v\n", err)
		return 1
	}

	shutdownContext, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()

	shutdownErr := false
	if routerService != nil {
		if err := routerService.Shutdown(shutdownContext); err != nil && !errors.Is(err, context.Canceled) {
			fmt.Fprintf(stderr, "agentation router shutdown failed: %v\n", err)
			shutdownErr = true
		}
	}

	if serverService != nil {
		if err := serverService.Shutdown(shutdownContext); err != nil && !errors.Is(err, context.Canceled) {
			fmt.Fprintf(stderr, "agentation server shutdown failed: %v\n", err)
			shutdownErr = true
		}
	}

	if shutdownErr {
		return 1
	}

	return 0
}
