package commands

import (
	"context"
	"flag"
	"fmt"
	"io"
	"time"

	"github.com/benjitaylor/agentation/cli/internal/api"
)

func RunWatch(ctx context.Context, client *api.Client, args []string, stdout, stderr io.Writer) error {
	projectID, flagArgs, err := parseRequiredLeadingArg(args, "watch <project-id> [--batch-window 10] [--timeout 300] [--json]")
	if err != nil {
		return err
	}

	flags := flag.NewFlagSet("watch", flag.ContinueOnError)
	flags.SetOutput(stderr)
	batchWindow := flags.Int("batch-window", 10, "Seconds to collect after first event (1-60)")
	timeout := flags.Int("timeout", 300, "Seconds to wait for first event (1-300)")
	asJSON := flags.Bool("json", false, "Output JSON")
	if err := flags.Parse(flagArgs); err != nil {
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
	printWatchAnnotations(stdout, output.Annotations)
	return nil
}
