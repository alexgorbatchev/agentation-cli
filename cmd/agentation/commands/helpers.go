package commands

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/alexgorbatchev/agentation-cli/internal/api"
)

func parseRequiredLeadingArg(args []string, usage string) (string, []string, error) {
	if len(args) == 0 {
		return "", nil, fmt.Errorf("usage: %s", usage)
	}

	value := strings.TrimSpace(args[0])
	if value == "" || strings.HasPrefix(value, "-") {
		return "", nil, fmt.Errorf("usage: %s", usage)
	}

	return value, args[1:], nil
}

func writeJSON(writer io.Writer, value any) error {
	encoder := json.NewEncoder(writer)
	encoder.SetIndent("", "  ")
	return encoder.Encode(value)
}

func writef(writer io.Writer, format string, args ...any) error {
	if _, err := fmt.Fprintf(writer, format, args...); err != nil {
		return fmt.Errorf("writing output: %w", err)
	}
	return nil
}

func writeln(writer io.Writer, args ...any) error {
	if _, err := fmt.Fprintln(writer, args...); err != nil {
		return fmt.Errorf("writing output: %w", err)
	}
	return nil
}

func printPendingAnnotations(writer io.Writer, annotations []api.Annotation) error {
	for idx, ann := range annotations {
		if err := writef(writer, "[%d] %s\n", idx+1, ann.ID); err != nil {
			return err
		}
		if err := writef(writer, "    %s\n", ann.Comment); err != nil {
			return err
		}
		if ann.Element != "" {
			if err := writef(writer, "    Element: %s\n", ann.Element); err != nil {
				return err
			}
		}
	}
	return nil
}

func printWatchAnnotations(writer io.Writer, annotations []api.Annotation) error {
	for idx, ann := range annotations {
		if err := writef(writer, "[%d] %s\n", idx+1, ann.ID); err != nil {
			return err
		}
		if err := writef(writer, "    %s\n", ann.Comment); err != nil {
			return err
		}
		if ann.SessionID != "" {
			if err := writef(writer, "    Session: %s\n", ann.SessionID); err != nil {
				return err
			}
		}
	}
	return nil
}
