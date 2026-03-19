package main

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestRunGenerateFixLoopSkill(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := run([]string{"generate", "--fix-loop-skill"}, &stdout, &stderr)
	if exitCode != 0 {
		t.Fatalf("run() exitCode = %d, want 0; stderr = %q", exitCode, stderr.String())
	}

	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}

	output := stdout.String()
	mustContain(t, output, "name: agentation-fix-loop")
	mustContain(t, output, "# Agentation Fix Loop (CLI)")
}

func TestRunGenerateUsageErrors(t *testing.T) {
	tests := []struct {
		name          string
		args          []string
		wantErrSubstr string
	}{
		{
			name:          "missing required flag",
			args:          []string{"generate"},
			wantErrSubstr: "usage: generate --fix-loop-skill",
		},
		{
			name:          "unexpected positional arg",
			args:          []string{"generate", "--fix-loop-skill", "extra"},
			wantErrSubstr: "usage: generate --fix-loop-skill",
		},
		{
			name:          "unknown flag",
			args:          []string{"generate", "--not-a-flag"},
			wantErrSubstr: "flag provided but not defined",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var stdout bytes.Buffer
			var stderr bytes.Buffer

			exitCode := run(tc.args, &stdout, &stderr)
			if exitCode != 1 {
				t.Fatalf("run(%v) exitCode = %d, want 1", tc.args, exitCode)
			}

			if stdout.Len() != 0 {
				t.Fatalf("stdout = %q, want empty", stdout.String())
			}

			mustContain(t, stderr.String(), tc.wantErrSubstr)
		})
	}
}

func TestPrintUsageIncludesGenerateCommand(t *testing.T) {
	var output bytes.Buffer

	printUsage(&output)
	mustContain(t, output.String(), "generate --fix-loop-skill")
}

func TestEmbeddedFixLoopSkillMatchesSourceSkill(t *testing.T) {
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller returned ok=false")
	}

	sourcePath := filepath.Join(filepath.Dir(currentFile), "..", "..", "..", "skills", "agentation-fix-loop", "SKILL.md")
	sourceContent, err := os.ReadFile(sourcePath)
	if err != nil {
		t.Fatalf("reading %s: %v", sourcePath, err)
	}

	want := strings.TrimRight(string(sourceContent), "\n")
	got := strings.TrimRight(fixLoopSkillContent, "\n")
	if got != want {
		t.Fatalf("embedded fix-loop skill is out of sync with %s", sourcePath)
	}
}

func TestRunPendingRequiresProjectID(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	err := runPending(t.Context(), nil, []string{}, &stdout, &stderr)
	if err == nil || !strings.Contains(err.Error(), "usage: pending <project-id> [--json]") {
		t.Fatalf("expected required project usage error, got %v", err)
	}
}

func TestRunPendingRequiresProjectIDAsFirstArg(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	err := runPending(t.Context(), nil, []string{"--json", "project-1"}, &stdout, &stderr)
	if err == nil || !strings.Contains(err.Error(), "usage: pending <project-id> [--json]") {
		t.Fatalf("expected first-arg usage error, got %v", err)
	}
}

func TestRunWatchRequiresProjectID(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	err := runWatch(t.Context(), nil, []string{}, &stdout, &stderr)
	if err == nil || !strings.Contains(err.Error(), "usage: watch <project-id> [--batch-window 10] [--timeout 300] [--json]") {
		t.Fatalf("expected required project usage error, got %v", err)
	}
}

func TestRunWatchRequiresProjectIDAsFirstArg(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	err := runWatch(t.Context(), nil, []string{"--json", "project-1"}, &stdout, &stderr)
	if err == nil || !strings.Contains(err.Error(), "usage: watch <project-id> [--batch-window 10] [--timeout 300] [--json]") {
		t.Fatalf("expected first-arg usage error, got %v", err)
	}
}

func TestRunProjectRequiresProjectID(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	err := runProject(t.Context(), nil, []string{}, &stdout, &stderr)
	if err == nil || !strings.Contains(err.Error(), "usage: project <project-id> [--json]") {
		t.Fatalf("expected required project usage error, got %v", err)
	}
}

func TestRunProjectRequiresProjectIDAsFirstArg(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	err := runProject(t.Context(), nil, []string{"--json", "project-1"}, &stdout, &stderr)
	if err == nil || !strings.Contains(err.Error(), "usage: project <project-id> [--json]") {
		t.Fatalf("expected first-arg usage error, got %v", err)
	}
}

func TestRunRejectsSessionsCommand(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := run([]string{"sessions"}, &stdout, &stderr)
	if exitCode != 1 {
		t.Fatalf("run(sessions) exitCode = %d, want 1", exitCode)
	}
	if !strings.Contains(stderr.String(), "unknown command \"sessions\"") {
		t.Fatalf("stderr = %q, want unknown command", stderr.String())
	}
}

func TestRunRejectsSessionCommand(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := run([]string{"session", "s1"}, &stdout, &stderr)
	if exitCode != 1 {
		t.Fatalf("run(session) exitCode = %d, want 1", exitCode)
	}
	if !strings.Contains(stderr.String(), "unknown command \"session\"") {
		t.Fatalf("stderr = %q, want unknown command", stderr.String())
	}
}

func TestRunPendingRejectsBaseURLBeforeProjectID(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := run([]string{"pending", "--base-url", "http://localhost:4747", "project-1"}, &stdout, &stderr)
	if exitCode != 1 {
		t.Fatalf("run(pending with --base-url first) exitCode = %d, want 1", exitCode)
	}
	mustContain(t, stderr.String(), "usage: pending <project-id> [--base-url <url>] [--json]")
}

func TestRunAckRejectsBaseURLBeforeAnnotationID(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := run([]string{"ack", "--base-url", "http://localhost:4747", "ann_1"}, &stdout, &stderr)
	if exitCode != 1 {
		t.Fatalf("run(ack with --base-url first) exitCode = %d, want 1", exitCode)
	}
	mustContain(t, stderr.String(), "usage: ack <annotation-id> [--base-url <url>] [--json]")
}

func TestPrintUsageIncludesProjectsCommand(t *testing.T) {
	var output bytes.Buffer

	printUsage(&output)
	mustContain(t, output.String(), "projects [--base-url <url>]")
	mustContain(t, output.String(), "project <project-id> [--base-url <url>] [--json]")
	mustContain(t, output.String(), "pending <project-id> [--base-url <url>] [--json]")
	mustContain(t, output.String(), "watch <project-id> [--base-url <url>] [--batch-window 10]")
	mustContain(t, output.String(), "[--timeout 300] [--json]")
}

func mustContain(t *testing.T, text, want string) {
	t.Helper()
	if !strings.Contains(text, want) {
		t.Fatalf("text did not contain %q\n--- text ---\n%s", want, text)
	}
}
