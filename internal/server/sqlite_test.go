package server

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestSQLiteBackendRoundTrip(t *testing.T) {
	t.Setenv("AGENTATION_STORE", "sqlite")
	t.Setenv("AGENTATION_DB_PATH", filepath.Join(t.TempDir(), "store.db"))

	backend, err := newPersistenceBackend()
	if err != nil {
		t.Fatalf("newPersistenceBackend error: %v", err)
	}
	if backend == nil {
		t.Fatal("expected sqlite backend")
	}
	defer backend.Close()

	session := Session{
		ID:        "s1",
		URL:       "http://example.com",
		Status:    "active",
		CreatedAt: nowISO(),
		Metadata:  map[string]any{"env": "dev"},
	}
	if err := backend.UpsertSession(session); err != nil {
		t.Fatalf("UpsertSession error: %v", err)
	}

	annotation := Annotation{
		ID:          "a1",
		SessionID:   session.ID,
		Comment:     "Fix this",
		Element:     "button",
		ElementPath: "body > button",
		Status:      StatusPending,
		CreatedAt:   nowISO(),
	}
	if err := backend.UpsertAnnotation(annotation); err != nil {
		t.Fatalf("UpsertAnnotation error: %v", err)
	}

	event := Event{Type: EventAnnotationCreated, SessionID: session.ID, Sequence: 1, Payload: annotation}
	if err := backend.InsertEvent(event); err != nil {
		t.Fatalf("InsertEvent error: %v", err)
	}

	snapshot, err := backend.LoadSnapshot()
	if err != nil {
		t.Fatalf("LoadSnapshot error: %v", err)
	}
	if len(snapshot.Sessions) != 1 || len(snapshot.Annotations) != 1 {
		t.Fatalf("unexpected snapshot sizes: sessions=%d annotations=%d", len(snapshot.Sessions), len(snapshot.Annotations))
	}
	if snapshot.Sequence != 1 {
		t.Fatalf("snapshot sequence = %d, want 1", snapshot.Sequence)
	}

	if err := backend.DeleteAnnotation(annotation.ID); err != nil {
		t.Fatalf("DeleteAnnotation error: %v", err)
	}
	postDelete, err := backend.LoadSnapshot()
	if err != nil {
		t.Fatalf("LoadSnapshot after delete error: %v", err)
	}
	if len(postDelete.Annotations) != 0 {
		t.Fatalf("expected no annotations after delete, got %d", len(postDelete.Annotations))
	}

	if err := backend.InsertEvent(Event{Sequence: 0, SessionID: session.ID}); err == nil {
		t.Fatal("InsertEvent should fail when sequence is zero")
	}

	if err := backend.Close(); err != nil {
		t.Fatalf("Close error: %v", err)
	}
}

func TestSQLiteModesAndPaths(t *testing.T) {
	t.Setenv("AGENTATION_STORE", "memory")
	backend, err := newPersistenceBackend()
	if err != nil {
		t.Fatalf("newPersistenceBackend(memory) error: %v", err)
	}
	if backend != nil {
		t.Fatal("memory mode should return nil backend")
	}

	customPath := filepath.Join(t.TempDir(), "custom.db")
	t.Setenv("AGENTATION_STORE", "sqlite")
	t.Setenv("AGENTATION_DB_PATH", customPath)

	resolved, err := sqlitePath()
	if err != nil {
		t.Fatalf("sqlitePath error: %v", err)
	}
	if resolved != customPath {
		t.Fatalf("sqlitePath = %q, want %q", resolved, customPath)
	}

	backend, err = newPersistenceBackend()
	if err != nil {
		t.Fatalf("newPersistenceBackend(sqlite) error: %v", err)
	}
	if backend == nil {
		t.Fatal("sqlite mode should create backend")
	}
	_ = backend.Close()

	t.Setenv("AGENTATION_DB_PATH", "")
	xdgDataHome := filepath.Join(t.TempDir(), "xdg-data")
	t.Setenv("XDG_DATA_HOME", xdgDataHome)
	xdgPath, err := sqlitePath()
	if err != nil {
		t.Fatalf("sqlitePath xdg error: %v", err)
	}
	if xdgPath != filepath.Join(xdgDataHome, "agentation", "store.db") {
		t.Fatalf("unexpected xdg sqlite path: %q", xdgPath)
	}

	t.Setenv("XDG_DATA_HOME", "")
	defaultPath, err := sqlitePath()
	if err != nil {
		t.Fatalf("sqlitePath default error: %v", err)
	}
	if !strings.Contains(defaultPath, filepath.Join(".local", "share", "agentation")) || !strings.HasSuffix(defaultPath, "store.db") {
		t.Fatalf("unexpected default sqlite path: %q", defaultPath)
	}

	if got := emptyToNil(" "); got != nil {
		t.Fatalf("emptyToNil should return nil for blank string, got %#v", got)
	}
	if got := emptyToNil("x"); got != "x" {
		t.Fatalf("emptyToNil should preserve non-empty value, got %#v", got)
	}
}

func TestSQLiteLoadSnapshotErrors(t *testing.T) {
	t.Setenv("AGENTATION_STORE", "sqlite")
	t.Setenv("AGENTATION_DB_PATH", filepath.Join(t.TempDir(), "bad.db"))

	backendAny, err := newPersistenceBackend()
	if err != nil {
		t.Fatalf("newPersistenceBackend error: %v", err)
	}
	backend := backendAny.(*sqliteBackend)
	defer backend.Close()

	_, err = backend.db.Exec(`INSERT INTO sessions (id, url, status, created_at) VALUES ('s1', 'http://example.com', 'active', ?)`, nowISO())
	if err != nil {
		t.Fatalf("insert session error: %v", err)
	}

	_, err = backend.db.Exec(`INSERT INTO annotations (id, session_id, data_json) VALUES ('a1', 's1', '{bad-json')`)
	if err != nil {
		t.Fatalf("insert invalid annotation row error: %v", err)
	}

	if _, err := backend.LoadSnapshot(); err == nil {
		t.Fatal("LoadSnapshot should fail for invalid annotation JSON")
	}
}

func TestSQLiteCloseNilBackend(t *testing.T) {
	var backend *sqliteBackend
	if err := backend.Close(); err != nil {
		t.Fatalf("nil backend Close should not fail: %v", err)
	}

	nonNil := &sqliteBackend{}
	if err := nonNil.Close(); err != nil {
		t.Fatalf("backend with nil db Close should not fail: %v", err)
	}
}
