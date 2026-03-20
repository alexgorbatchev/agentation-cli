package commands

import (
	"context"
	"flag"
	"fmt"
	"io"
	"sort"
	"strings"
	"sync"
	"time"

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

const projectSessionFetchConcurrency = 8
const recentProjectActivityWindow = 24 * time.Hour

// ProjectSessionFetchWorkerLimit exposes the concurrency ceiling for session detail fetches.
const ProjectSessionFetchWorkerLimit = projectSessionFetchConcurrency

// ProjectSummary is the JSON payload returned by the `project --json` command.
type ProjectSummary = projectSummary

type projectSessionJob struct {
	index   int
	session api.Session
}

type projectSessionResult struct {
	index   int
	summary projectSessionSummary
	err     error
}

func recentProjectIDs(sessions []api.Session, now time.Time) []string {
	cutoff := now.Add(-recentProjectActivityWindow)
	projectSet := make(map[string]struct{})
	for _, session := range sessions {
		projectID := strings.TrimSpace(session.ProjectID)
		if projectID == "" {
			continue
		}

		activityTime, ok := sessionActivityTime(session)
		if !ok || activityTime.Before(cutoff) {
			continue
		}

		projectSet[projectID] = struct{}{}
	}

	projects := make([]string, 0, len(projectSet))
	for projectID := range projectSet {
		projects = append(projects, projectID)
	}
	sort.Strings(projects)
	return projects
}

func sessionActivityTime(session api.Session) (time.Time, bool) {
	for _, timestamp := range []string{session.UpdatedAt, session.CreatedAt} {
		trimmedTimestamp := strings.TrimSpace(timestamp)
		if trimmedTimestamp == "" {
			continue
		}

		activityTime, err := time.Parse(time.RFC3339Nano, trimmedTimestamp)
		if err == nil {
			return activityTime, true
		}
	}

	return time.Time{}, false
}

func fetchProjectSessionSummaries(ctx context.Context, client *api.Client, sessions []api.Session) ([]projectSessionSummary, int, error) {
	if len(sessions) == 0 {
		return nil, 0, nil
	}

	workerCount := projectSessionFetchConcurrency
	if workerCount > len(sessions) {
		workerCount = len(sessions)
	}

	jobChannel := make(chan projectSessionJob, len(sessions))
	for index, session := range sessions {
		jobChannel <- projectSessionJob{index: index, session: session}
	}
	close(jobChannel)

	resultChannel := make(chan projectSessionResult, len(sessions))

	var workerGroup sync.WaitGroup
	worker := func() {
		defer workerGroup.Done()
		for job := range jobChannel {
			sessionWithAnnotations, err := client.GetSession(ctx, job.session.ID)
			if err != nil {
				resultChannel <- projectSessionResult{index: job.index, err: err}
				continue
			}

			resultChannel <- projectSessionResult{
				index: job.index,
				summary: projectSessionSummary{
					ID:              job.session.ID,
					Status:          job.session.Status,
					URL:             job.session.URL,
					AnnotationCount: len(sessionWithAnnotations.Annotations),
				},
			}
		}
	}

	workerGroup.Add(workerCount)
	for workerIndex := 0; workerIndex < workerCount; workerIndex++ {
		go worker()
	}

	go func() {
		workerGroup.Wait()
		close(resultChannel)
	}()

	summaries := make([]projectSessionSummary, len(sessions))
	totalAnnotations := 0
	errorsByIndex := make(map[int]error)

	for result := range resultChannel {
		if result.err != nil {
			errorsByIndex[result.index] = result.err
			continue
		}

		summaries[result.index] = result.summary
		totalAnnotations += result.summary.AnnotationCount
	}

	for index := range sessions {
		if err, ok := errorsByIndex[index]; ok {
			return nil, 0, err
		}
	}

	return summaries, totalAnnotations, nil
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

	projects := recentProjectIDs(sessions, time.Now().UTC())

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

	projectSessions, totalAnnotations, err := fetchProjectSessionSummaries(ctx, client, sessions)
	if err != nil {
		return err
	}

	summary := projectSummary{
		ProjectID:       projectID,
		AnnotationCount: totalAnnotations,
		Sessions:        projectSessions,
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
