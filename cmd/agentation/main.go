package main

import (
	"context"
	_ "embed"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/benjitaylor/agentation/cli/internal/api"
	"github.com/benjitaylor/agentation/cli/internal/lifecycle"
)

//go:embed embedded/agentation-fix-loop-skill.md
var fixLoopSkillContent string

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
	case "projects":
		return runWithAPICommand(ctx, commandArgs, stdout, stderr, runProjects)
	case "project":
		return runWithFirstPositionalAPICommand(ctx, commandArgs, stdout, stderr, runProject, "project <project-id> [--base-url <url>] [--json]")
	case "pending":
		return runWithFirstPositionalAPICommand(ctx, commandArgs, stdout, stderr, runPending, "pending <project-id> [--base-url <url>] [--json]")
	case "ack":
		return runWithFirstPositionalAPICommand(ctx, commandArgs, stdout, stderr, runAcknowledge, "ack <annotation-id> [--base-url <url>] [--json]")
	case "resolve":
		return runWithFirstPositionalAPICommand(ctx, commandArgs, stdout, stderr, runResolve, "resolve <annotation-id> [--base-url <url>] [--summary text] [--json]")
	case "dismiss":
		return runWithFirstPositionalAPICommand(ctx, commandArgs, stdout, stderr, runDismiss, "dismiss <annotation-id> [--base-url <url>] --reason text [--json]")
	case "reply":
		return runWithFirstPositionalAPICommand(ctx, commandArgs, stdout, stderr, runReply, "reply <annotation-id> [--base-url <url>] --message text [--json]")
	case "watch":
		return runWithFirstPositionalAPICommand(ctx, commandArgs, stdout, stderr, runWatch, "watch <project-id> [--base-url <url>] [--batch-window 10] [--timeout 300] [--json]")
	case "generate":
		return runGenerate(commandArgs, stdout, stderr)
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

func runWithFirstPositionalAPICommand(ctx context.Context, args []string, stdout, stderr io.Writer, runner apiCommandRunner, usage string) int {
	if len(args) == 0 || strings.TrimSpace(args[0]) == "" || strings.HasPrefix(strings.TrimSpace(args[0]), "-") {
		fmt.Fprintf(stderr, "error: usage: %s\n", usage)
		return 1
	}

	return runWithAPICommand(ctx, args, stdout, stderr, runner)
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

func runProjects(ctx context.Context, client *api.Client, args []string, stdout, stderr io.Writer) error {
	flags := flag.NewFlagSet("projects", flag.ContinueOnError)
	flags.SetOutput(stderr)
	asJSON := flags.Bool("json", false, "Output JSON")
	if err := flags.Parse(args); err != nil {
		return err
	}

	sessions, err := client.ListSessions(ctx, "")
	if err != nil {
		return err
	}

	projectSet := make(map[string]struct{})
	for _, session := range sessions {
		projectID := strings.TrimSpace(session.ProjectID)
		if projectID == "" {
			continue
		}
		projectSet[projectID] = struct{}{}
	}

	projects := make([]string, 0, len(projectSet))
	for projectID := range projectSet {
		projects = append(projects, projectID)
	}
	sort.Strings(projects)

	if *asJSON {
		return writeJSON(stdout, projects)
	}

	if len(projects) == 0 {
		fmt.Fprintln(stdout, "No projects found.")
		return nil
	}

	for _, projectID := range projects {
		fmt.Fprintln(stdout, projectID)
	}

	return nil
}

type projectSummary struct {
	ProjectID       string                  `json:"projectId"`
	SessionCount    int                     `json:"sessionCount"`
	AnnotationCount int                     `json:"annotationCount"`
	Sessions        []projectSessionSummary `json:"sessions"`
}

type projectSessionSummary struct {
	ID              string `json:"id"`
	Status          string `json:"status"`
	URL             string `json:"url"`
	AnnotationCount int    `json:"annotationCount"`
}

func runProject(ctx context.Context, client *api.Client, args []string, stdout, stderr io.Writer) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: project <project-id> [--json]")
	}

	projectID := strings.TrimSpace(args[0])
	if projectID == "" || strings.HasPrefix(projectID, "-") {
		return fmt.Errorf("usage: project <project-id> [--json]")
	}

	flags := flag.NewFlagSet("project", flag.ContinueOnError)
	flags.SetOutput(stderr)
	asJSON := flags.Bool("json", false, "Output JSON")
	if err := flags.Parse(args[1:]); err != nil {
		return err
	}
	if flags.NArg() > 0 {
		return fmt.Errorf("usage: project <project-id> [--json]")
	}

	sessions, err := client.ListSessions(ctx, projectID)
	if err != nil {
		return err
	}

	summary := projectSummary{
		ProjectID: projectID,
		Sessions:  make([]projectSessionSummary, 0, len(sessions)),
	}

	for _, session := range sessions {
		sessionWithAnnotations, err := client.GetSession(ctx, session.ID)
		if err != nil {
			return err
		}
		annotationCount := len(sessionWithAnnotations.Annotations)
		summary.AnnotationCount += annotationCount
		summary.Sessions = append(summary.Sessions, projectSessionSummary{
			ID:              session.ID,
			Status:          session.Status,
			URL:             session.URL,
			AnnotationCount: annotationCount,
		})
	}

	sort.Slice(summary.Sessions, func(i, j int) bool {
		return summary.Sessions[i].ID < summary.Sessions[j].ID
	})
	summary.SessionCount = len(summary.Sessions)

	if *asJSON {
		return writeJSON(stdout, summary)
	}

	fmt.Fprintf(stdout, "Project: %s\n", summary.ProjectID)
	fmt.Fprintf(stdout, "Sessions: %d\n", summary.SessionCount)
	fmt.Fprintf(stdout, "Annotations: %d\n", summary.AnnotationCount)
	if summary.SessionCount == 0 {
		return nil
	}

	for _, session := range summary.Sessions {
		fmt.Fprintf(stdout, "%s\t%s\t%s\tannotations=%d\n", session.ID, session.Status, session.URL, session.AnnotationCount)
	}

	return nil
}

func runPending(ctx context.Context, client *api.Client, args []string, stdout, stderr io.Writer) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: pending <project-id> [--json]")
	}

	projectID := strings.TrimSpace(args[0])
	if projectID == "" || strings.HasPrefix(projectID, "-") {
		return fmt.Errorf("usage: pending <project-id> [--json]")
	}

	flags := flag.NewFlagSet("pending", flag.ContinueOnError)
	flags.SetOutput(stderr)
	asJSON := flags.Bool("json", false, "Output JSON")
	if err := flags.Parse(args[1:]); err != nil {
		return err
	}
	if flags.NArg() > 0 {
		return fmt.Errorf("usage: pending <project-id> [--json]")
	}

	pending, err := client.GetPending(ctx, "", projectID)
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
	if len(args) == 0 {
		return fmt.Errorf("usage: ack <annotation-id> [--json]")
	}

	annotationID := strings.TrimSpace(args[0])
	if annotationID == "" || strings.HasPrefix(annotationID, "-") {
		return fmt.Errorf("usage: ack <annotation-id> [--json]")
	}

	flags := flag.NewFlagSet("ack", flag.ContinueOnError)
	flags.SetOutput(stderr)
	asJSON := flags.Bool("json", false, "Output JSON")
	if err := flags.Parse(args[1:]); err != nil {
		return err
	}
	if flags.NArg() > 0 {
		return fmt.Errorf("usage: ack <annotation-id> [--json]")
	}

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
	if len(args) == 0 {
		return fmt.Errorf("usage: resolve <annotation-id> [--summary text] [--json]")
	}

	annotationID := strings.TrimSpace(args[0])
	if annotationID == "" || strings.HasPrefix(annotationID, "-") {
		return fmt.Errorf("usage: resolve <annotation-id> [--summary text] [--json]")
	}

	flags := flag.NewFlagSet("resolve", flag.ContinueOnError)
	flags.SetOutput(stderr)
	summary := flags.String("summary", "", "Optional resolution summary")
	asJSON := flags.Bool("json", false, "Output JSON")
	if err := flags.Parse(args[1:]); err != nil {
		return err
	}
	if flags.NArg() > 0 {
		return fmt.Errorf("usage: resolve <annotation-id> [--summary text] [--json]")
	}

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
	if len(args) == 0 {
		return fmt.Errorf("usage: dismiss <annotation-id> --reason text [--json]")
	}

	annotationID := strings.TrimSpace(args[0])
	if annotationID == "" || strings.HasPrefix(annotationID, "-") {
		return fmt.Errorf("usage: dismiss <annotation-id> --reason text [--json]")
	}

	flags := flag.NewFlagSet("dismiss", flag.ContinueOnError)
	flags.SetOutput(stderr)
	reason := flags.String("reason", "", "Dismissal reason (required)")
	asJSON := flags.Bool("json", false, "Output JSON")
	if err := flags.Parse(args[1:]); err != nil {
		return err
	}
	if flags.NArg() > 0 {
		return fmt.Errorf("usage: dismiss <annotation-id> --reason text [--json]")
	}
	if strings.TrimSpace(*reason) == "" {
		return fmt.Errorf("dismiss requires --reason")
	}

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
	if len(args) == 0 {
		return fmt.Errorf("usage: reply <annotation-id> --message text [--json]")
	}

	annotationID := strings.TrimSpace(args[0])
	if annotationID == "" || strings.HasPrefix(annotationID, "-") {
		return fmt.Errorf("usage: reply <annotation-id> --message text [--json]")
	}

	flags := flag.NewFlagSet("reply", flag.ContinueOnError)
	flags.SetOutput(stderr)
	message := flags.String("message", "", "Reply message (required)")
	asJSON := flags.Bool("json", false, "Output JSON")
	if err := flags.Parse(args[1:]); err != nil {
		return err
	}
	if flags.NArg() > 0 {
		return fmt.Errorf("usage: reply <annotation-id> --message text [--json]")
	}
	if strings.TrimSpace(*message) == "" {
		return fmt.Errorf("reply requires --message")
	}

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
	if len(args) == 0 {
		return fmt.Errorf("usage: watch <project-id> [--batch-window 10] [--timeout 300] [--json]")
	}

	projectID := strings.TrimSpace(args[0])
	if projectID == "" || strings.HasPrefix(projectID, "-") {
		return fmt.Errorf("usage: watch <project-id> [--batch-window 10] [--timeout 300] [--json]")
	}

	flags := flag.NewFlagSet("watch", flag.ContinueOnError)
	flags.SetOutput(stderr)
	batchWindow := flags.Int("batch-window", 10, "Seconds to collect after first event (1-60)")
	timeout := flags.Int("timeout", 300, "Seconds to wait for first event (1-300)")
	asJSON := flags.Bool("json", false, "Output JSON")
	if err := flags.Parse(args[1:]); err != nil {
		return err
	}
	if flags.NArg() > 0 {
		return fmt.Errorf("usage: watch <project-id> [--batch-window 10] [--timeout 300] [--json]")
	}

	output, err := client.Watch(ctx, api.WatchOptions{
		ProjectID:   projectID,
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

func runGenerate(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("generate", flag.ContinueOnError)
	flags.SetOutput(stderr)
	fixLoopSkill := flags.Bool("fix-loop-skill", false, "Print embedded Agentation fix-loop skill markdown")
	if err := flags.Parse(args); err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 1
	}

	if flags.NArg() > 0 {
		fmt.Fprintln(stderr, "error: usage: generate --fix-loop-skill")
		return 1
	}

	if !*fixLoopSkill {
		fmt.Fprintln(stderr, "error: usage: generate --fix-loop-skill")
		return 1
	}

	if _, err := io.WriteString(stdout, strings.TrimRight(fixLoopSkillContent, "\n")+"\n"); err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 1
	}

	return 0
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
	commands := []struct {
		usage             string
		usageContinuation string
		description       string
	}{
		{"ack <annotation-id> [--base-url <url>] [--json]", "", "Mark annotation acknowledged"},
		{"dismiss <annotation-id> [--base-url <url>] --reason text [--json]", "", "Dismiss annotation"},
		{"generate --fix-loop-skill", "", "Print embedded Agentation fix-loop skill"},
		{"pending <project-id> [--base-url <url>] [--json]", "", "Get pending annotations"},
		{"project <project-id> [--base-url <url>] [--json]", "", "Get project summary"},
		{"projects [--base-url <url>]", "", "List project IDs"},
		{"reply <annotation-id> [--base-url <url>] --message text [--json]", "", "Add thread reply"},
		{"resolve <annotation-id> [--base-url <url>] [--summary text] [--json]", "", "Resolve annotation"},
		{"start", "", "Start local services (single PID)"},
		{"status", "", "Show local service status"},
		{"stop", "", "Stop local services (single PID)"},
		{"watch <project-id> [--base-url <url>] [--batch-window 10]", "[--timeout 300] [--json]", "Wait for new annotations/thread replies"},
	}

	maxUsageLength := 0
	for _, command := range commands {
		if len(command.usage) > maxUsageLength {
			maxUsageLength = len(command.usage)
		}
	}

	for _, command := range commands {
		fmt.Fprintf(writer, "  %-*s %s\n", maxUsageLength, command.usage, command.description)
		if command.usageContinuation != "" {
			continuationIndent := strings.Index(command.usage, "[")
			if continuationIndent < 0 {
				continuationIndent = len(command.usage) + 1
			}
			fmt.Fprintf(writer, "  %*s%s\n", continuationIndent, "", command.usageContinuation)
		}
	}
	fmt.Fprintln(writer)
	fmt.Fprintln(writer, "Examples:")
	fmt.Fprintln(writer, "  agentation start")
	fmt.Fprintln(writer, "  AGENTATION_SERVER_ADDR=127.0.0.1:5757 AGENTATION_ROUTER_ADDR=127.0.0.1:8787 agentation start")
	fmt.Fprintln(writer, "  AGENTATION_SERVER_ADDR=0 agentation start")
	fmt.Fprintln(writer, "  AGENTATION_ROUTER_ADDR=0 agentation start")
	fmt.Fprintln(writer, "  agentation start --server-addr 127.0.0.1:4747 --router-addr 127.0.0.1:8787")
	fmt.Fprintln(writer, "  AGENTATION_BASE_URL=http://127.0.0.1:4747 agentation projects --json")
	fmt.Fprintln(writer, "  agentation project my-project --json")
	fmt.Fprintln(writer, "  agentation pending my-project --json")
	fmt.Fprintln(writer, "  agentation watch my-project --batch-window 5 --timeout 300 --json")
	fmt.Fprintln(writer, "  agentation ack ann_123 --base-url http://127.0.0.1:4747")
	fmt.Fprintln(writer, "  agentation resolve ann_123 --summary \"Updated spacing\"")
	fmt.Fprintln(writer, "  agentation generate --fix-loop-skill")
}
