package server

import (
	"io"
	"testing"
)

func closeTestCloser(t *testing.T, closer io.Closer, name string) {
	t.Helper()
	if closer == nil {
		return
	}
	if err := closer.Close(); err != nil {
		t.Errorf("close %s: %v", name, err)
	}
}
