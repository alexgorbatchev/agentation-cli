package commands

import (
	"context"
	"flag"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/benjitaylor/agentation/cli/internal/api"
)

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

func RunProjects(ctx context.Context, client *api.Client, args []string, stdout, stderr io.Writer) error {
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

func RunProject(ctx context.Context, client *api.Client, args []string, stdout, stderr io.Writer) error {
	projectID, flagArgs, err := parseRequiredLeadingArg(args, "project <project-id> [--json]")
	if err != nil {
		return err
	}

	flags := flag.NewFlagSet("project", flag.ContinueOnError)
	flags.SetOutput(stderr)
	asJSON := flags.Bool("json", false, "Output JSON")
	if err := flags.Parse(flagArgs); err != nil {
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
