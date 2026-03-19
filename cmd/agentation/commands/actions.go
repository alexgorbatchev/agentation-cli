package commands

import (
	"context"
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/benjitaylor/agentation/cli/internal/api"
)

func RunAcknowledge(ctx context.Context, client *api.Client, args []string, stdout, stderr io.Writer) error {
	annotationID, flagArgs, err := parseRequiredLeadingArg(args, "ack <annotation-id> [--json]")
	if err != nil {
		return err
	}

	flags := flag.NewFlagSet("ack", flag.ContinueOnError)
	flags.SetOutput(stderr)
	asJSON := flags.Bool("json", false, "Output JSON")
	if err := flags.Parse(flagArgs); err != nil {
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

func RunResolve(ctx context.Context, client *api.Client, args []string, stdout, stderr io.Writer) error {
	annotationID, flagArgs, err := parseRequiredLeadingArg(args, "resolve <annotation-id> [--summary text] [--json]")
	if err != nil {
		return err
	}

	flags := flag.NewFlagSet("resolve", flag.ContinueOnError)
	flags.SetOutput(stderr)
	summary := flags.String("summary", "", "Optional resolution summary")
	asJSON := flags.Bool("json", false, "Output JSON")
	if err := flags.Parse(flagArgs); err != nil {
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

func RunDismiss(ctx context.Context, client *api.Client, args []string, stdout, stderr io.Writer) error {
	annotationID, flagArgs, err := parseRequiredLeadingArg(args, "dismiss <annotation-id> --reason text [--json]")
	if err != nil {
		return err
	}

	flags := flag.NewFlagSet("dismiss", flag.ContinueOnError)
	flags.SetOutput(stderr)
	reason := flags.String("reason", "", "Dismissal reason (required)")
	asJSON := flags.Bool("json", false, "Output JSON")
	if err := flags.Parse(flagArgs); err != nil {
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

func RunReply(ctx context.Context, client *api.Client, args []string, stdout, stderr io.Writer) error {
	annotationID, flagArgs, err := parseRequiredLeadingArg(args, "reply <annotation-id> --message text [--json]")
	if err != nil {
		return err
	}

	flags := flag.NewFlagSet("reply", flag.ContinueOnError)
	flags.SetOutput(stderr)
	message := flags.String("message", "", "Reply message (required)")
	asJSON := flags.Bool("json", false, "Output JSON")
	if err := flags.Parse(flagArgs); err != nil {
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
