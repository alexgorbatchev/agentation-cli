package commands

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/benjitaylor/agentation/cli/internal/api"
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

func printPendingAnnotations(writer io.Writer, annotations []api.Annotation) {
	for idx, ann := range annotations {
		fmt.Fprintf(writer, "[%d] %s\n", idx+1, ann.ID)
		fmt.Fprintf(writer, "    %s\n", ann.Comment)
		if ann.Element != "" {
			fmt.Fprintf(writer, "    Element: %s\n", ann.Element)
		}
	}
}

func printWatchAnnotations(writer io.Writer, annotations []api.Annotation) {
	for idx, ann := range annotations {
		fmt.Fprintf(writer, "[%d] %s\n", idx+1, ann.ID)
		fmt.Fprintf(writer, "    %s\n", ann.Comment)
		if ann.SessionID != "" {
			fmt.Fprintf(writer, "    Session: %s\n", ann.SessionID)
		}
	}
}
