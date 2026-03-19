package lifecycle

import (
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"
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
	return runStartCommand(args, stdout, stderr)
}

func RunServe(args []string, stdout, stderr io.Writer) int {
	return runServeCommand(args, stdout, stderr)
}

func RunStop(args []string, stdout, stderr io.Writer) int {
	return runStopCommand(args, stdout, stderr)
}

func RunStatus(args []string, stdout, stderr io.Writer) int {
	return runStatusCommand(args, stdout, stderr)
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
