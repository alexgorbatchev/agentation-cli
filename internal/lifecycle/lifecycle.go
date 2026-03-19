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
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/benjitaylor/agentation/cli/internal/procctl"

	routerconfig "github.com/benjitaylor/agentation/cli/internal/router/config"
	routerhttp "github.com/benjitaylor/agentation/cli/internal/router/http"
	routerpkg "github.com/benjitaylor/agentation/cli/internal/router/router"
	routerstore "github.com/benjitaylor/agentation/cli/internal/router/store"
	"github.com/benjitaylor/agentation/cli/internal/server"
)

const (
	defaultServerAddress = "127.0.0.1:4747"
	defaultRouterAddress = "127.0.0.1:8787"
	shutdownTimeout      = 5 * time.Second
)

type serveConfig struct {
	serverAddr   string
	routerAddr   string
	enableServer bool
	enableRouter bool
}

type startConfig struct {
	foreground bool
	serve      serveConfig
}

func RunStart(args []string, stdout, stderr io.Writer) int {
	cfg, err := parseStartFlags(args, stderr)
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		fmt.Fprintf(stderr, "failed to parse start flags: %v\n", err)
		return 1
	}

	controller := stackController()
	if pid, ok := controller.LoadRunningPID(); ok {
		fmt.Fprintf(stdout, "agentation already running (pid %d)\n", pid)
		return 0
	}

	if cfg.foreground {
		return runServe(cfg.serve, stdout, stderr)
	}

	stackLogPath := stackLogFilePath()
	pid, err := controller.StartBackground("__serve-stack", []string{
		"--server-addr", cfg.serve.serverAddr,
		"--router-addr", cfg.serve.routerAddr,
	}, stackLogPath)
	if err != nil {
		fmt.Fprintf(stderr, "failed to start agentation: %v\n", err)
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

func RunServe(args []string, stdout, stderr io.Writer) int {
	cfg, err := parseServeFlags(args, stderr)
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		fmt.Fprintf(stderr, "failed to parse serve flags: %v\n", err)
		return 1
	}

	return runServe(cfg, stdout, stderr)
}

func runServe(cfg serveConfig, stdout, stderr io.Writer) int {
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

func RunStop(args []string, stdout, stderr io.Writer) int {
	if err := parseNoArgCommand("stop", args, stderr); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		fmt.Fprintf(stderr, "failed to parse stop flags: %v\n", err)
		return 1
	}

	controller := stackController()
	pid, stopped, err := controller.Stop(30, 100*time.Millisecond)
	if err != nil {
		fmt.Fprintf(stderr, "failed to stop agentation: %v\n", err)
		return 1
	}
	if !stopped {
		fmt.Fprintln(stdout, "agentation is not running")
		return 0
	}

	fmt.Fprintf(stdout, "agentation stopped (pid %d)\n", pid)
	return 0
}

func RunStatus(args []string, stdout, stderr io.Writer) int {
	if err := parseNoArgCommand("status", args, stderr); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		fmt.Fprintf(stderr, "failed to parse status flags: %v\n", err)
		return 1
	}

	pid, ok := stackController().LoadRunningPID()
	if !ok {
		fmt.Fprintln(stdout, "agentation not running")
		return 1
	}

	fmt.Fprintf(stdout, "agentation running (pid %d)\n", pid)
	return 0
}

func parseStartFlags(args []string, stderr io.Writer) (startConfig, error) {
	flags := flag.NewFlagSet("agentation start", flag.ContinueOnError)
	flags.SetOutput(stderr)
	flags.Usage = func() {
		fmt.Fprintln(stderr, "Usage: agentation start [--server-addr host:port|0] [--router-addr host:port|0] [--foreground|--background]")
		fmt.Fprintln(stderr)
		fmt.Fprintln(stderr, "Options:")
		flags.PrintDefaults()
		fmt.Fprintln(stderr)
		fmt.Fprintln(stderr, "Examples:")
		fmt.Fprintln(stderr, "  agentation start")
		fmt.Fprintln(stderr, "  AGENTATION_SERVER_ADDR=0 agentation start")
		fmt.Fprintln(stderr, "  AGENTATION_ROUTER_ADDR=0 agentation start")
		fmt.Fprintln(stderr, "  agentation start --server-addr 127.0.0.1:4747 --router-addr 127.0.0.1:8787")
	}

	serverAddrFlag := flags.String("server-addr", "", "Server address (default: AGENTATION_SERVER_ADDR or 127.0.0.1:4747; use 0 to disable)")
	routerAddrFlag := flags.String("router-addr", "", "Router address (default: AGENTATION_ROUTER_ADDR or 127.0.0.1:8787; use 0 to disable)")
	foreground := flags.Bool("foreground", false, "Run in foreground")
	background := flags.Bool("background", false, "Run in background (default)")

	if err := flags.Parse(args); err != nil {
		return startConfig{}, err
	}
	if flags.NArg() != 0 {
		return startConfig{}, fmt.Errorf("start does not accept positional arguments")
	}
	if *foreground && *background {
		return startConfig{}, fmt.Errorf("--foreground and --background cannot be used together")
	}

	serve, err := resolveServeConfig(strings.TrimSpace(*serverAddrFlag), strings.TrimSpace(*routerAddrFlag))
	if err != nil {
		return startConfig{}, err
	}

	return startConfig{
		foreground: *foreground,
		serve:      serve,
	}, nil
}

func parseServeFlags(args []string, stderr io.Writer) (serveConfig, error) {
	flags := flag.NewFlagSet("agentation __serve-stack", flag.ContinueOnError)
	flags.SetOutput(stderr)

	serverAddrFlag := flags.String("server-addr", "", "Server address")
	routerAddrFlag := flags.String("router-addr", "", "Router address")

	if err := flags.Parse(args); err != nil {
		return serveConfig{}, err
	}
	if flags.NArg() != 0 {
		return serveConfig{}, fmt.Errorf("serve does not accept positional arguments")
	}

	return resolveServeConfig(strings.TrimSpace(*serverAddrFlag), strings.TrimSpace(*routerAddrFlag))
}

func resolveServeConfig(serverAddrFlag string, routerAddrFlag string) (serveConfig, error) {
	serverAddr := resolveAddress(serverAddrFlag, strings.TrimSpace(os.Getenv("AGENTATION_SERVER_ADDR")), defaultServerAddress)
	routerAddr := resolveAddress(routerAddrFlag, firstNonEmptyEnv("AGENTATION_ROUTER_ADDR", "AGENTATION_ROUTER_ADDRESS"), defaultRouterAddress)

	cfg := serveConfig{
		serverAddr:   serverAddr,
		routerAddr:   routerAddr,
		enableServer: serverAddr != "0",
		enableRouter: routerAddr != "0",
	}

	if !cfg.enableServer && !cfg.enableRouter {
		return serveConfig{}, fmt.Errorf("both server and router are disabled; set AGENTATION_SERVER_ADDR and/or AGENTATION_ROUTER_ADDR to a listen address")
	}

	return cfg, nil
}

func resolveAddress(flagValue string, envValue string, fallback string) string {
	if flagValue != "" {
		return flagValue
	}
	if envValue != "" {
		return envValue
	}
	return fallback
}

func firstNonEmptyEnv(keys ...string) string {
	for _, key := range keys {
		value := strings.TrimSpace(os.Getenv(key))
		if value != "" {
			return value
		}
	}
	return ""
}

func parseNoArgCommand(commandName string, args []string, stderr io.Writer) error {
	flags := flag.NewFlagSet("agentation "+commandName, flag.ContinueOnError)
	flags.SetOutput(stderr)
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("%s does not accept positional arguments", commandName)
	}
	return nil
}

func pidFilePath() string {
	return procctl.PathFromEnv("AGENTATION_PID_FILE", "agentation.pid")
}

func stackLogFilePath() string {
	return procctl.PathFromEnv("AGENTATION_LOG_FILE", "agentation.log")
}

func serverLogFilePath() string {
	return procctl.PathFromEnv("AGENTATION_SERVER_LOG_FILE", "agentation-server.log")
}

func routerLogFilePath() string {
	return procctl.PathFromEnv("AGENTATION_ROUTER_LOG_FILE", "agentation-router.log")
}

func openServiceLogWriter(path string, stdout io.Writer) (io.Writer, *os.File, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, nil, err
	}

	file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, nil, err
	}

	writer := io.Writer(file)
	if stdout != nil {
		writer = io.MultiWriter(stdout, file)
	}

	return writer, file, nil
}

func stackController() procctl.Controller {
	return procctl.New(pidFilePath(), "__serve-stack")
}
