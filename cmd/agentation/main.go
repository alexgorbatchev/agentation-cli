package main

import (
	"context"
	_ "embed"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/benjitaylor/agentation/cli/cmd/agentation/commands"
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
	return commands.RunProjects(ctx, client, args, stdout, stderr)
}

func runProject(ctx context.Context, client *api.Client, args []string, stdout, stderr io.Writer) error {
	return commands.RunProject(ctx, client, args, stdout, stderr)
}

func runPending(ctx context.Context, client *api.Client, args []string, stdout, stderr io.Writer) error {
	return commands.RunPending(ctx, client, args, stdout, stderr)
}

func runAcknowledge(ctx context.Context, client *api.Client, args []string, stdout, stderr io.Writer) error {
	return commands.RunAcknowledge(ctx, client, args, stdout, stderr)
}

func runResolve(ctx context.Context, client *api.Client, args []string, stdout, stderr io.Writer) error {
	return commands.RunResolve(ctx, client, args, stdout, stderr)
}

func runDismiss(ctx context.Context, client *api.Client, args []string, stdout, stderr io.Writer) error {
	return commands.RunDismiss(ctx, client, args, stdout, stderr)
}

func runReply(ctx context.Context, client *api.Client, args []string, stdout, stderr io.Writer) error {
	return commands.RunReply(ctx, client, args, stdout, stderr)
}

func runWatch(ctx context.Context, client *api.Client, args []string, stdout, stderr io.Writer) error {
	return commands.RunWatch(ctx, client, args, stdout, stderr)
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
