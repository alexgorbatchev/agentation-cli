package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/benjitaylor/agentation/cli/internal/api"
	"github.com/benjitaylor/agentation/cli/internal/servercmd"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	cfg, remaining, err := parseGlobalFlags(args, stderr)
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			printUsage(stdout)
			return 0
		}
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 1
	}

	if len(remaining) == 0 {
		printUsage(stdout)
		return 0
	}

	client := api.NewClient(cfg.baseURL)
	ctx := context.Background()

	command := remaining[0]
	commandArgs := remaining[1:]

	switch command {
	case "sessions":
		if err := runSessions(ctx, client, commandArgs, stdout, stderr); err != nil {
			fmt.Fprintf(stderr, "error: %v\n", err)
			return 1
		}
	case "session":
		if err := runSession(ctx, client, commandArgs, stdout, stderr); err != nil {
			fmt.Fprintf(stderr, "error: %v\n", err)
			return 1
		}
	case "pending":
		if err := runPending(ctx, client, commandArgs, stdout, stderr); err != nil {
			fmt.Fprintf(stderr, "error: %v\n", err)
			return 1
		}
	case "ack":
		if err := runAcknowledge(ctx, client, commandArgs, stdout, stderr); err != nil {
			fmt.Fprintf(stderr, "error: %v\n", err)
			return 1
		}
	case "resolve":
		if err := runResolve(ctx, client, commandArgs, stdout, stderr); err != nil {
			fmt.Fprintf(stderr, "error: %v\n", err)
			return 1
		}
	case "dismiss":
		if err := runDismiss(ctx, client, commandArgs, stdout, stderr); err != nil {
			fmt.Fprintf(stderr, "error: %v\n", err)
			return 1
		}
	case "reply":
		if err := runReply(ctx, client, commandArgs, stdout, stderr); err != nil {
			fmt.Fprintf(stderr, "error: %v\n", err)
			return 1
		}
	case "watch":
		if err := runWatch(ctx, client, commandArgs, stdout, stderr); err != nil {
			fmt.Fprintf(stderr, "error: %v\n", err)
			return 1
		}
	case "server":
		return servercmd.Run(commandArgs, stdout, stderr)
	case "help", "--help", "-h":
		printUsage(stdout)
	default:
		fmt.Fprintf(stderr, "error: unknown command %q\n\n", command)
		printUsage(stderr)
		return 1
	}

	return 0
}

type globalConfig struct {
	baseURL string
}

func parseGlobalFlags(args []string, stderr io.Writer) (globalConfig, []string, error) {
	cfg := globalConfig{
		baseURL: strings.TrimSpace(os.Getenv("AGENTATION_HTTP_URL")),
	}

	flags := flag.NewFlagSet("agentation", flag.ContinueOnError)
	flags.SetOutput(stderr)
	flags.StringVar(&cfg.baseURL, "base-url", cfg.baseURL, "Agentation HTTP base URL (default: http://localhost:4747)")

	if err := flags.Parse(args); err != nil {
		return globalConfig{}, nil, err
	}

	if strings.TrimSpace(cfg.baseURL) == "" {
		cfg.baseURL = "http://localhost:4747"
	}

	return cfg, flags.Args(), nil
}

func runSessions(ctx context.Context, client *api.Client, args []string, stdout, stderr io.Writer) error {
	flags := flag.NewFlagSet("sessions", flag.ContinueOnError)
	flags.SetOutput(stderr)
	asJSON := flags.Bool("json", false, "Output JSON")
	if err := flags.Parse(args); err != nil {
		return err
	}

	sessions, err := client.ListSessions(ctx)
	if err != nil {
		return err
	}

	if *asJSON {
		return writeJSON(stdout, sessions)
	}

	if len(sessions) == 0 {
		fmt.Fprintln(stdout, "No active sessions.")
		return nil
	}

	for _, session := range sessions {
		fmt.Fprintf(stdout, "%s\t%s\t%s\n", session.ID, session.Status, session.URL)
	}

	return nil
}

func runSession(ctx context.Context, client *api.Client, args []string, stdout, stderr io.Writer) error {
	flags := flag.NewFlagSet("session", flag.ContinueOnError)
	flags.SetOutput(stderr)
	asJSON := flags.Bool("json", false, "Output JSON")
	if err := flags.Parse(args); err != nil {
		return err
	}

	if flags.NArg() != 1 {
		return fmt.Errorf("usage: session [--json] <session-id>")
	}

	sessionID := flags.Arg(0)
	session, err := client.GetSession(ctx, sessionID)
	if err != nil {
		return err
	}

	if *asJSON {
		return writeJSON(stdout, session)
	}

	fmt.Fprintf(stdout, "Session: %s\n", session.ID)
	fmt.Fprintf(stdout, "URL: %s\n", session.URL)
	fmt.Fprintf(stdout, "Status: %s\n", session.Status)
	fmt.Fprintf(stdout, "Annotations: %d\n", len(session.Annotations))
	return nil
}

func runPending(ctx context.Context, client *api.Client, args []string, stdout, stderr io.Writer) error {
	flags := flag.NewFlagSet("pending", flag.ContinueOnError)
	flags.SetOutput(stderr)
	sessionID := flags.String("session", "", "Filter by session ID")
	asJSON := flags.Bool("json", false, "Output JSON")
	if err := flags.Parse(args); err != nil {
		return err
	}

	pending, err := client.GetPending(ctx, strings.TrimSpace(*sessionID))
	if err != nil {
		return err
	}

	if *asJSON {
		return writeJSON(stdout, pending)
	}

	fmt.Fprintf(stdout, "Pending annotations: %d\n", pending.Count)
	for idx, ann := range pending.Annotations {
		fmt.Fprintf(stdout, "[%d] %s\n", idx+1, ann.ID)
		fmt.Fprintf(stdout, "    %s\n", ann.Comment)
		if ann.Element != "" {
			fmt.Fprintf(stdout, "    Element: %s\n", ann.Element)
		}
	}

	return nil
}

func runAcknowledge(ctx context.Context, client *api.Client, args []string, stdout, stderr io.Writer) error {
	flags := flag.NewFlagSet("ack", flag.ContinueOnError)
	flags.SetOutput(stderr)
	asJSON := flags.Bool("json", false, "Output JSON")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 1 {
		return fmt.Errorf("usage: ack [--json] <annotation-id>")
	}

	annotationID := flags.Arg(0)
	if err := client.Acknowledge(ctx, annotationID); err != nil {
		return err
	}

	result := map[string]any{"acknowledged": true, "annotationId": annotationID}
	if *asJSON {
		return writeJSON(stdout, result)
	}

	fmt.Fprintf(stdout, "Acknowledged %s\n", annotationID)
	return nil
}

func runResolve(ctx context.Context, client *api.Client, args []string, stdout, stderr io.Writer) error {
	flags := flag.NewFlagSet("resolve", flag.ContinueOnError)
	flags.SetOutput(stderr)
	summary := flags.String("summary", "", "Optional resolution summary")
	asJSON := flags.Bool("json", false, "Output JSON")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 1 {
		return fmt.Errorf("usage: resolve [--summary text] [--json] <annotation-id>")
	}

	annotationID := flags.Arg(0)
	if err := client.Resolve(ctx, annotationID, *summary); err != nil {
		return err
	}

	result := map[string]any{"resolved": true, "annotationId": annotationID}
	if strings.TrimSpace(*summary) != "" {
		result["summary"] = strings.TrimSpace(*summary)
	}

	if *asJSON {
		return writeJSON(stdout, result)
	}

	fmt.Fprintf(stdout, "Resolved %s\n", annotationID)
	return nil
}

func runDismiss(ctx context.Context, client *api.Client, args []string, stdout, stderr io.Writer) error {
	flags := flag.NewFlagSet("dismiss", flag.ContinueOnError)
	flags.SetOutput(stderr)
	reason := flags.String("reason", "", "Dismissal reason (required)")
	asJSON := flags.Bool("json", false, "Output JSON")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 1 {
		return fmt.Errorf("usage: dismiss --reason text [--json] <annotation-id>")
	}
	if strings.TrimSpace(*reason) == "" {
		return fmt.Errorf("dismiss requires --reason")
	}

	annotationID := flags.Arg(0)
	if err := client.Dismiss(ctx, annotationID, *reason); err != nil {
		return err
	}

	result := map[string]any{"dismissed": true, "annotationId": annotationID, "reason": strings.TrimSpace(*reason)}
	if *asJSON {
		return writeJSON(stdout, result)
	}

	fmt.Fprintf(stdout, "Dismissed %s\n", annotationID)
	return nil
}

func runReply(ctx context.Context, client *api.Client, args []string, stdout, stderr io.Writer) error {
	flags := flag.NewFlagSet("reply", flag.ContinueOnError)
	flags.SetOutput(stderr)
	message := flags.String("message", "", "Reply message (required)")
	asJSON := flags.Bool("json", false, "Output JSON")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 1 {
		return fmt.Errorf("usage: reply --message text [--json] <annotation-id>")
	}
	if strings.TrimSpace(*message) == "" {
		return fmt.Errorf("reply requires --message")
	}

	annotationID := flags.Arg(0)
	if err := client.Reply(ctx, annotationID, *message); err != nil {
		return err
	}

	result := map[string]any{"replied": true, "annotationId": annotationID, "message": strings.TrimSpace(*message)}
	if *asJSON {
		return writeJSON(stdout, result)
	}

	fmt.Fprintf(stdout, "Replied to %s\n", annotationID)
	return nil
}

func runWatch(ctx context.Context, client *api.Client, args []string, stdout, stderr io.Writer) error {
	flags := flag.NewFlagSet("watch", flag.ContinueOnError)
	flags.SetOutput(stderr)
	sessionID := flags.String("session", "", "Optional session ID filter")
	batchWindow := flags.Int("batch-window", 10, "Seconds to collect after first event (1-60)")
	timeout := flags.Int("timeout", 120, "Seconds to wait for first event (1-300)")
	asJSON := flags.Bool("json", false, "Output JSON")
	if err := flags.Parse(args); err != nil {
		return err
	}

	output, err := client.Watch(ctx, api.WatchOptions{
		SessionID:   strings.TrimSpace(*sessionID),
		BatchWindow: time.Duration(*batchWindow) * time.Second,
		Timeout:     time.Duration(*timeout) * time.Second,
	})
	if err != nil {
		return err
	}

	if *asJSON {
		return writeJSON(stdout, output)
	}

	if output.Timeout {
		fmt.Fprintln(stdout, output.Message)
		return nil
	}

	fmt.Fprintf(stdout, "Received %d annotation(s)\n", output.Count)
	for idx, ann := range output.Annotations {
		fmt.Fprintf(stdout, "[%d] %s\n", idx+1, ann.ID)
		fmt.Fprintf(stdout, "    %s\n", ann.Comment)
		if ann.SessionID != "" {
			fmt.Fprintf(stdout, "    Session: %s\n", ann.SessionID)
		}
	}
	return nil
}

func writeJSON(writer io.Writer, value any) error {
	encoder := json.NewEncoder(writer)
	encoder.SetIndent("", "  ")
	return encoder.Encode(value)
}

func printUsage(writer io.Writer) {
	fmt.Fprintln(writer, "agentation - CLI companion for Agentation HTTP server")
	fmt.Fprintln(writer)
	fmt.Fprintln(writer, "Global options:")
	fmt.Fprintln(writer, "  --base-url <url>   Agentation HTTP base URL (default: http://localhost:4747)")
	fmt.Fprintln(writer)
	fmt.Fprintln(writer, "Commands:")
	fmt.Fprintln(writer, "  sessions                         List sessions")
	fmt.Fprintln(writer, "  session <session-id>             Get session with annotations")
	fmt.Fprintln(writer, "  pending [--session <id>]         Get pending annotations")
	fmt.Fprintln(writer, "  ack <annotation-id>              Mark annotation acknowledged")
	fmt.Fprintln(writer, "  resolve <annotation-id>          Resolve annotation")
	fmt.Fprintln(writer, "  dismiss <annotation-id>          Dismiss annotation")
	fmt.Fprintln(writer, "  reply <annotation-id>            Add thread reply")
	fmt.Fprintln(writer, "  watch                            Wait for new annotations/thread replies")
	fmt.Fprintln(writer, "  server                           Manage local Agentation HTTP server")
	fmt.Fprintln(writer)
	fmt.Fprintln(writer, "Examples:")
	fmt.Fprintln(writer, "  agentation pending --json")
	fmt.Fprintln(writer, "  agentation ack ann_123")
	fmt.Fprintln(writer, "  agentation resolve ann_123 --summary \"Updated spacing\"")
	fmt.Fprintln(writer, "  agentation watch --batch-window 5 --timeout 120 --json")
}
