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

func mustContain(t *testing.T, text, want string) {
	t.Helper()
	if !strings.Contains(text, want) {
		t.Fatalf("text did not contain %q\n--- text ---\n%s", want, text)
	}
}
