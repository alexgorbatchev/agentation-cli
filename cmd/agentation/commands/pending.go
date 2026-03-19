package commands

import (
	"context"
	"flag"
	"fmt"
	"io"

	"github.com/benjitaylor/agentation/cli/internal/api"
)

func RunPending(ctx context.Context, client *api.Client, args []string, stdout, stderr io.Writer) error {
	projectID, flagArgs, err := parseRequiredLeadingArg(args, "pending <project-id> [--json]")
	if err != nil {
		return err
	}

	flags := flag.NewFlagSet("pending", flag.ContinueOnError)
	flags.SetOutput(stderr)
	asJSON := flags.Bool("json", false, "Output JSON")
	if err := flags.Parse(flagArgs); err != nil {
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
	printPendingAnnotations(stdout, pending.Annotations)
	return nil
}
