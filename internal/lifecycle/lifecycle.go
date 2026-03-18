package lifecycle

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/benjitaylor/agentation/cli/internal/routerctl"
	"github.com/benjitaylor/agentation/cli/internal/serverctl"
)

const (
	defaultServerAddress = "127.0.0.1:4747"
	defaultRouterAddress = "127.0.0.1:8787"
)

type startConfig struct {
	foreground bool
	server     bool
	serverAddr string
	router     bool
	routerAddr string
}

type controlConfig struct {
	server bool
	router bool
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

	if cfg.foreground {
		if cfg.server && cfg.router {
			fmt.Fprintln(stderr, "error: --foreground cannot run server and router together; use --background")
			return 1
		}

		if cfg.server {
			return serverctl.Run([]string{"serve", "--address", cfg.serverAddr}, stdout, stderr)
		}

		return routerctl.Run([]string{"serve", "--address", cfg.routerAddr}, stdout, stderr)
	}

	startedServer := false
	if cfg.server {
		code := serverctl.Run([]string{"start", "--background", "--address", cfg.serverAddr}, stdout, stderr)
		if code != 0 {
			return code
		}
		startedServer = true
	}

	if cfg.router {
		code := routerctl.Run([]string{"start", "--background", "--address", cfg.routerAddr}, stdout, stderr)
		if code != 0 {
			if startedServer {
				_ = serverctl.Run([]string{"stop"}, io.Discard, io.Discard)
			}
			return code
		}
	}

	return 0
}

func RunStop(args []string, stdout, stderr io.Writer) int {
	cfg, err := parseControlFlags("stop", args, stderr)
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		fmt.Fprintf(stderr, "failed to parse stop flags: %v\n", err)
		return 1
	}

	exitCode := 0
	if cfg.router {
		code := routerctl.Run([]string{"stop"}, stdout, stderr)
		if code != 0 {
			exitCode = 1
		}
	}

	if cfg.server {
		code := serverctl.Run([]string{"stop"}, stdout, stderr)
		if code != 0 {
			exitCode = 1
		}
	}

	return exitCode
}

func RunStatus(args []string, stdout, stderr io.Writer) int {
	cfg, err := parseControlFlags("status", args, stderr)
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		fmt.Fprintf(stderr, "failed to parse status flags: %v\n", err)
		return 1
	}

	exitCode := 0
	if cfg.server {
		code := serverctl.Run([]string{"status"}, stdout, stderr)
		if code != 0 {
			exitCode = 1
		}
	}

	if cfg.router {
		code := routerctl.Run([]string{"status"}, stdout, stderr)
		if code != 0 {
			exitCode = 1
		}
	}

	return exitCode
}

func parseStartFlags(args []string, stderr io.Writer) (startConfig, error) {
	serverAddrFromEnv := strings.TrimSpace(os.Getenv("AGENTATION_SERVER_ADDR"))
	routerAddrFromEnv := strings.TrimSpace(os.Getenv("AGENTATION_ROUTER_ADDR"))

	flags := flag.NewFlagSet("agentation start", flag.ContinueOnError)
	flags.SetOutput(stderr)
	flags.Usage = func() {
		fmt.Fprintln(stderr, "Usage: agentation start [--server] [--server-addr host:port] [--router] [--router-addr host:port] [--foreground|--background]")
		fmt.Fprintln(stderr)
		fmt.Fprintln(stderr, "Options:")
		flags.PrintDefaults()
		fmt.Fprintln(stderr)
		fmt.Fprintln(stderr, "Examples:")
		fmt.Fprintln(stderr, "  agentation start")
		fmt.Fprintln(stderr, "  AGENTATION_SERVER_ADDR=127.0.0.1:5757 agentation start")
		fmt.Fprintln(stderr, "  AGENTATION_ROUTER_ADDR=127.0.0.1:8787 agentation start")
		fmt.Fprintln(stderr, "  agentation start --server --server-addr 127.0.0.1:4747 --router --router-addr 127.0.0.1:8787")
	}

	server := flags.Bool("server", false, "Start Agentation HTTP server")
	serverAddr := flags.String("server-addr", "", "HTTP server address (default: AGENTATION_SERVER_ADDR or 127.0.0.1:4747)")
	router := flags.Bool("router", false, "Start Agentation router")
	routerAddr := flags.String("router-addr", "", "Router address (default: AGENTATION_ROUTER_ADDR or 127.0.0.1:8787)")
	foreground := flags.Bool("foreground", false, "Run selected service in foreground")
	background := flags.Bool("background", false, "Run selected service in background (default)")

	if err := flags.Parse(args); err != nil {
		return startConfig{}, err
	}
	if flags.NArg() != 0 {
		return startConfig{}, fmt.Errorf("start does not accept positional arguments")
	}
	if *foreground && *background {
		return startConfig{}, fmt.Errorf("--foreground and --background cannot be used together")
	}

	serverAddrValue := strings.TrimSpace(*serverAddr)
	routerAddrValue := strings.TrimSpace(*routerAddr)

	hasServiceSelection := *server || *router || serverAddrValue != "" || routerAddrValue != ""

	cfg := startConfig{}
	if hasServiceSelection {
		cfg.server = *server || serverAddrValue != ""
		cfg.router = *router || routerAddrValue != ""
	} else {
		cfg.server = true
		cfg.router = routerAddrFromEnv != ""
	}

	if !cfg.server && !cfg.router {
		return startConfig{}, fmt.Errorf("no service selected; pass --server or --router")
	}

	if cfg.server {
		cfg.serverAddr = defaultServerAddress
		if serverAddrFromEnv != "" {
			cfg.serverAddr = serverAddrFromEnv
		}
		if serverAddrValue != "" {
			cfg.serverAddr = serverAddrValue
		}
	}

	if cfg.router {
		cfg.routerAddr = defaultRouterAddress
		if routerAddrFromEnv != "" {
			cfg.routerAddr = routerAddrFromEnv
		}
		if routerAddrValue != "" {
			cfg.routerAddr = routerAddrValue
		}
	}

	cfg.foreground = *foreground
	return cfg, nil
}

func parseControlFlags(commandName string, args []string, stderr io.Writer) (controlConfig, error) {
	flags := flag.NewFlagSet("agentation "+commandName, flag.ContinueOnError)
	flags.SetOutput(stderr)

	server := flags.Bool("server", false, commandName+" server")
	router := flags.Bool("router", false, commandName+" router")

	if err := flags.Parse(args); err != nil {
		return controlConfig{}, err
	}
	if flags.NArg() != 0 {
		return controlConfig{}, fmt.Errorf("%s does not accept positional arguments", commandName)
	}

	cfg := controlConfig{server: *server, router: *router}
	if cfg.server || cfg.router {
		return cfg, nil
	}

	cfg.server = true
	cfg.router = strings.TrimSpace(os.Getenv("AGENTATION_ROUTER_ADDR")) != ""
	return cfg, nil
}
