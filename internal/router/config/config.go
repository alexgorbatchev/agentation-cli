package config

import (
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Address            string
	AuthToken          string
	RequestBodyLimit   int64
	ForwardTimeout     time.Duration
	ReadTimeout        time.Duration
	WriteTimeout       time.Duration
	ReadHeaderTimeout  time.Duration
	IdleTimeout        time.Duration
	SessionStaleAfter  time.Duration
	AllowAbsolutePaths bool
	EnforceRootBounds  bool
}

func Load(args []string, stderr io.Writer) (Config, error) {
	defaults := Config{
		Address:            envOrDefault("AGENTATION_ROUTER_ADDRESS", "127.0.0.1:8787"),
		AuthToken:          strings.TrimSpace(os.Getenv("AGENTATION_ROUTER_TOKEN")),
		RequestBodyLimit:   envInt64OrDefault("AGENTATION_ROUTER_BODY_LIMIT", 1024*1024),
		ForwardTimeout:     envDurationOrDefault("AGENTATION_ROUTER_FORWARD_TIMEOUT", 1500*time.Millisecond),
		ReadTimeout:        envDurationOrDefault("AGENTATION_ROUTER_READ_TIMEOUT", 3*time.Second),
		WriteTimeout:       envDurationOrDefault("AGENTATION_ROUTER_WRITE_TIMEOUT", 3*time.Second),
		ReadHeaderTimeout:  envDurationOrDefault("AGENTATION_ROUTER_READ_HEADER_TIMEOUT", 2*time.Second),
		IdleTimeout:        envDurationOrDefault("AGENTATION_ROUTER_IDLE_TIMEOUT", 30*time.Second),
		SessionStaleAfter:  envDurationOrDefault("AGENTATION_ROUTER_SESSION_STALE_AFTER", 20*time.Second),
		AllowAbsolutePaths: envBoolOrDefault("AGENTATION_ROUTER_ALLOW_ABSOLUTE_PATHS", false),
		EnforceRootBounds:  envBoolOrDefault("AGENTATION_ROUTER_ENFORCE_ROOT_BOUNDS", true),
	}

	flags := flag.NewFlagSet("agentation-router", flag.ContinueOnError)
	flags.SetOutput(stderr)
	address := flags.String("address", defaults.Address, "listen address (host:port)")
	authToken := flags.String("token", defaults.AuthToken, "shared auth token for mutating endpoints")
	requestBodyLimit := flags.Int64("body-limit", defaults.RequestBodyLimit, "max request body size in bytes")
	forwardTimeout := flags.Duration("forward-timeout", defaults.ForwardTimeout, "timeout for forwarding requests to Neovim sessions")
	readTimeout := flags.Duration("read-timeout", defaults.ReadTimeout, "HTTP server read timeout")
	writeTimeout := flags.Duration("write-timeout", defaults.WriteTimeout, "HTTP server write timeout")
	readHeaderTimeout := flags.Duration("read-header-timeout", defaults.ReadHeaderTimeout, "HTTP server read header timeout")
	idleTimeout := flags.Duration("idle-timeout", defaults.IdleTimeout, "HTTP server idle timeout")
	sessionStaleAfter := flags.Duration("session-stale-after", defaults.SessionStaleAfter, "duration before inactive sessions are pruned")
	allowAbsolutePaths := flags.Bool("allow-absolute-paths", defaults.AllowAbsolutePaths, "allow absolute file paths on /open")
	enforceRootBounds := flags.Bool("enforce-root-bounds", defaults.EnforceRootBounds, "require absolute /open paths to stay under session root")

	if err := flags.Parse(args); err != nil {
		return Config{}, err
	}
	if flags.NArg() != 0 {
		return Config{}, fmt.Errorf("unexpected positional arguments: %v", flags.Args())
	}

	return Config{
		Address:            strings.TrimSpace(*address),
		AuthToken:          strings.TrimSpace(*authToken),
		RequestBodyLimit:   *requestBodyLimit,
		ForwardTimeout:     *forwardTimeout,
		ReadTimeout:        *readTimeout,
		WriteTimeout:       *writeTimeout,
		ReadHeaderTimeout:  *readHeaderTimeout,
		IdleTimeout:        *idleTimeout,
		SessionStaleAfter:  *sessionStaleAfter,
		AllowAbsolutePaths: *allowAbsolutePaths,
		EnforceRootBounds:  *enforceRootBounds,
	}, nil
}

func envOrDefault(key string, fallback string) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	return value
}

func envInt64OrDefault(key string, fallback int64) int64 {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	parsed, error := strconv.ParseInt(raw, 10, 64)
	if error != nil {
		return fallback
	}
	return parsed
}

func envDurationOrDefault(key string, fallback time.Duration) time.Duration {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	parsed, error := time.ParseDuration(raw)
	if error != nil {
		return fallback
	}
	return parsed
}

func envBoolOrDefault(key string, fallback bool) bool {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	parsed, error := strconv.ParseBool(raw)
	if error != nil {
		return fallback
	}
	return parsed
}
