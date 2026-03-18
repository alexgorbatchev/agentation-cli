package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/benjitaylor/agentation/cli/internal/api"
	"github.com/benjitaylor/agentation/cli/internal/lifecycle"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		printUsage(stdout)
		return 0
	}

	command := args[0]
	commandArgs := args[1:]
	ctx := context.Background()

	switch command {
	case "sessions":
		return runWithAPICommand(ctx, commandArgs, stdout, stderr, runSessions)
	case "session":
		return runWithAPICommand(ctx, commandArgs, stdout, stderr, runSession)
	case "pending":
		return runWithAPICommand(ctx, commandArgs, stdout, stderr, runPending)
	case "ack":
		return runWithAPICommand(ctx, commandArgs, stdout, stderr, runAcknowledge)
	case "resolve":
		return runWithAPICommand(ctx, commandArgs, stdout, stderr, runResolve)
	case "dismiss":
		return runWithAPICommand(ctx, commandArgs, stdout, stderr, runDismiss)
	case "reply":
		return runWithAPICommand(ctx, commandArgs, stdout, stderr, runReply)
	case "watch":
		return runWithAPICommand(ctx, commandArgs, stdout, stderr, runWatch)
	case "start":
		return lifecycle.RunStart(commandArgs, stdout, stderr)
	case "stop":
		return lifecycle.RunStop(commandArgs, stdout, stderr)
	case "status":
		return lifecycle.RunStatus(commandArgs, stdout, stderr)
	case "__serve-stack":
		return lifecycle.RunServe(commandArgs, stdout, stderr)
	case "help", "--help", "-h":
		printUsage(stdout)
		return 0
	default:
		fmt.Fprintf(stderr, "error: unknown command %q\n\n", command)
		printUsage(stderr)
		return 1
	}
}

type apiCommandRunner func(context.Context, *api.Client, []string, io.Writer, io.Writer) error

func runWithAPICommand(ctx context.Context, args []string, stdout, stderr io.Writer, runner apiCommandRunner) int {
	baseURL, remainingArgs, err := extractBaseURL(args)
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 1
	}

	client := api.NewClient(baseURL)
	if err := runner(ctx, client, remainingArgs, stdout, stderr); err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 1
	}

	return 0
}

func extractBaseURL(args []string) (string, []string, error) {
	baseURL := strings.TrimSpace(os.Getenv("AGENTATION_BASE_URL"))
	if baseURL == "" {
		baseURL = "http://localhost:4747"
	}

	remaining := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--base-url" {
			if i+1 >= len(args) {
				return "", nil, fmt.Errorf("--base-url requires a value")
			}
			i++
			value := strings.TrimSpace(args[i])
			if value == "" || strings.HasPrefix(value, "-") {
				return "", nil, fmt.Errorf("--base-url requires a valid URL value")
			}
			baseURL = value
			continue
		}

		if after, ok := strings.CutPrefix(arg, "--base-url="); ok {
			value := strings.TrimSpace(after)
			if value == "" {
				return "", nil, fmt.Errorf("--base-url requires a value")
			}
			baseURL = value
			continue
		}

		remaining = append(remaining, arg)
	}

	return baseURL, remaining, nil
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
	fmt.Fprintln(writer, "Commands:")
	fmt.Fprintln(writer, "  sessions [--base-url <url>]                         List sessions")
	fmt.Fprintln(writer, "  session [--base-url <url>] <session-id>             Get session with annotations")
	fmt.Fprintln(writer, "  pending [--base-url <url>] [--session <id>]         Get pending annotations")
	fmt.Fprintln(writer, "  ack [--base-url <url>] <annotation-id>              Mark annotation acknowledged")
	fmt.Fprintln(writer, "  resolve [--base-url <url>] <annotation-id>          Resolve annotation")
	fmt.Fprintln(writer, "  dismiss [--base-url <url>] <annotation-id>          Dismiss annotation")
	fmt.Fprintln(writer, "  reply [--base-url <url>] <annotation-id>            Add thread reply")
	fmt.Fprintln(writer, "  watch [--base-url <url>]                            Wait for new annotations/thread replies")
	fmt.Fprintln(writer, "  start                                                Start local services (single PID)")
	fmt.Fprintln(writer, "  stop                                                 Stop local services (single PID)")
	fmt.Fprintln(writer, "  status                                               Show local service status")
	fmt.Fprintln(writer)
	fmt.Fprintln(writer, "Examples:")
	fmt.Fprintln(writer, "  AGENTATION_BASE_URL=http://127.0.0.1:4747 agentation pending --json")
	fmt.Fprintln(writer, "  agentation pending --base-url http://127.0.0.1:4747 --json")
	fmt.Fprintln(writer, "  agentation ack --base-url http://127.0.0.1:4747 ann_123")
	fmt.Fprintln(writer, "  agentation resolve ann_123 --summary \"Updated spacing\"")
	fmt.Fprintln(writer, "  agentation watch --batch-window 5 --timeout 120 --json")
	fmt.Fprintln(writer, "  agentation start")
	fmt.Fprintln(writer, "  AGENTATION_SERVER_ADDR=127.0.0.1:5757 AGENTATION_ROUTER_ADDR=127.0.0.1:8787 agentation start")
	fmt.Fprintln(writer, "  AGENTATION_SERVER_ADDR=0 agentation start")
	fmt.Fprintln(writer, "  AGENTATION_ROUTER_ADDR=0 agentation start")
	fmt.Fprintln(writer, "  agentation start --server-addr 127.0.0.1:4747 --router-addr 127.0.0.1:8787")
}
