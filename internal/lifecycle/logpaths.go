package lifecycle

import (
	"io"
	"os"
	"path/filepath"
	"strings"
)

func stackLogFilePath() string {
	path := strings.TrimSpace(os.Getenv("AGENTATION_LOG_FILE"))
	if path != "" {
		return path
	}
	return filepath.Join(os.TempDir(), "agentation.log")
}

func serverLogFilePath() string {
	path := strings.TrimSpace(os.Getenv("AGENTATION_SERVER_LOG_FILE"))
	if path != "" {
		return path
	}
	return filepath.Join(os.TempDir(), "agentation-server.log")
}

func routerLogFilePath() string {
	path := strings.TrimSpace(os.Getenv("AGENTATION_ROUTER_LOG_FILE"))
	if path != "" {
		return path
	}
	return filepath.Join(os.TempDir(), "agentation-router.log")
}

func openServiceLogWriter(path string, stdout io.Writer) (io.Writer, *os.File, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, nil, err
	}

	file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, nil, err
	}

	writer := io.Writer(file)
	if stdout != nil {
		writer = io.MultiWriter(stdout, file)
	}

	return writer, file, nil
}
