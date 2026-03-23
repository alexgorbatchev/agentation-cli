package lifecycle

import (
	"fmt"
	"io"
)

func reportCloseError(writer io.Writer, name string, closer io.Closer) {
	if closer == nil {
		return
	}
	if err := closer.Close(); err != nil {
		_, _ = fmt.Fprintf(writer, "failed to close %s: %v\n", name, err)
	}
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
