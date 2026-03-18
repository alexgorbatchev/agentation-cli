package server

import (
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"

	_ "modernc.org/sqlite"
)

type storeSnapshot struct {
	Sessions    map[string]Session
	Annotations map[string]Annotation
	Events      map[string][]Event
	Sequence    int64
}

type persistenceBackend interface {
	LoadSnapshot() (storeSnapshot, error)
	UpsertSession(session Session) error
	UpsertAnnotation(annotation Annotation) error
	DeleteAnnotation(annotationID string) error
	InsertEvent(event Event) error
	Close() error
}

type sqliteBackend struct {
	db *sql.DB
}

func newPersistenceBackend() (persistenceBackend, error) {
	mode := strings.TrimSpace(strings.ToLower(os.Getenv("AGENTATION_STORE")))
	if mode == "memory" {
		return nil, nil
	}

	dbPath, err := sqlitePath()
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		return nil, err
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, err
	}

	backend := &sqliteBackend{db: db}
	if err := backend.init(); err != nil {
		_ = db.Close()
		return nil, err
	}

	return backend, nil
}

func sqlitePath() (string, error) {
	override := strings.TrimSpace(os.Getenv("AGENTATION_DB_PATH"))
	if override != "" {
		return override, nil
	}

	xdgDataHome := strings.TrimSpace(os.Getenv("XDG_DATA_HOME"))
	if xdgDataHome != "" {
		return filepath.Join(xdgDataHome, "agentation", "store.db"), nil
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".local", "share", "agentation", "store.db"), nil
}

func (b *sqliteBackend) init() error {
	statements := []string{
		"PRAGMA journal_mode=WAL;",
		"PRAGMA busy_timeout=5000;",
		"PRAGMA foreign_keys=ON;",
		`CREATE TABLE IF NOT EXISTS sessions (
			id TEXT PRIMARY KEY,
			url TEXT NOT NULL,
			status TEXT NOT NULL,
			created_at TEXT NOT NULL,
			updated_at TEXT,
			project_id TEXT,
			metadata_json TEXT
		);`,
		`CREATE TABLE IF NOT EXISTS annotations (
			id TEXT PRIMARY KEY,
			session_id TEXT NOT NULL,
			data_json TEXT NOT NULL,
			updated_at TEXT,
			FOREIGN KEY(session_id) REFERENCES sessions(id)
		);`,
		`CREATE TABLE IF NOT EXISTS events (
			sequence INTEGER PRIMARY KEY,
			session_id TEXT NOT NULL,
			data_json TEXT NOT NULL
		);`,
	}

	for _, statement := range statements {
		if _, err := b.db.Exec(statement); err != nil {
			return err
		}
	}

	return nil
}

func (b *sqliteBackend) LoadSnapshot() (storeSnapshot, error) {
	snapshot := storeSnapshot{
		Sessions:    make(map[string]Session),
		Annotations: make(map[string]Annotation),
		Events:      make(map[string][]Event),
	}

	sessionRows, err := b.db.Query(`SELECT id, url, status, created_at, COALESCE(updated_at, ''), COALESCE(project_id, ''), COALESCE(metadata_json, '') FROM sessions`)
	if err != nil {
		return storeSnapshot{}, err
	}
	defer sessionRows.Close()

	for sessionRows.Next() {
		var session Session
		var updatedAt string
		var projectID string
		var metadataJSON string
		if err := sessionRows.Scan(&session.ID, &session.URL, &session.Status, &session.CreatedAt, &updatedAt, &projectID, &metadataJSON); err != nil {
			return storeSnapshot{}, err
		}
		if updatedAt != "" {
			session.UpdatedAt = updatedAt
		}
		if projectID != "" {
			session.ProjectID = projectID
		}
		if metadataJSON != "" {
			_ = json.Unmarshal([]byte(metadataJSON), &session.Metadata)
		}
		snapshot.Sessions[session.ID] = session
	}
	if err := sessionRows.Err(); err != nil {
		return storeSnapshot{}, err
	}

	annotationRows, err := b.db.Query(`SELECT id, data_json FROM annotations`)
	if err != nil {
		return storeSnapshot{}, err
	}
	defer annotationRows.Close()

	for annotationRows.Next() {
		var annotationID string
		var dataJSON string
		if err := annotationRows.Scan(&annotationID, &dataJSON); err != nil {
			return storeSnapshot{}, err
		}
		var annotation Annotation
		if err := json.Unmarshal([]byte(dataJSON), &annotation); err != nil {
			return storeSnapshot{}, err
		}
		snapshot.Annotations[annotation.ID] = annotation
	}
	if err := annotationRows.Err(); err != nil {
		return storeSnapshot{}, err
	}

	eventRows, err := b.db.Query(`SELECT sequence, session_id, data_json FROM events ORDER BY sequence ASC`)
	if err != nil {
		return storeSnapshot{}, err
	}
	defer eventRows.Close()

	for eventRows.Next() {
		var sequence int64
		var sessionID string
		var dataJSON string
		if err := eventRows.Scan(&sequence, &sessionID, &dataJSON); err != nil {
			return storeSnapshot{}, err
		}
		var event Event
		if err := json.Unmarshal([]byte(dataJSON), &event); err != nil {
			return storeSnapshot{}, err
		}
		event.Sequence = sequence
		if event.SessionID == "" {
			event.SessionID = sessionID
		}
		snapshot.Events[event.SessionID] = append(snapshot.Events[event.SessionID], event)
		if sequence > snapshot.Sequence {
			snapshot.Sequence = sequence
		}
	}
	if err := eventRows.Err(); err != nil {
		return storeSnapshot{}, err
	}

	return snapshot, nil
}

func (b *sqliteBackend) UpsertSession(session Session) error {
	metadataJSON := ""
	if session.Metadata != nil {
		payload, err := json.Marshal(session.Metadata)
		if err != nil {
			return err
		}
		metadataJSON = string(payload)
	}

	_, err := b.db.Exec(
		`INSERT INTO sessions (id, url, status, created_at, updated_at, project_id, metadata_json)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			url = excluded.url,
			status = excluded.status,
			created_at = excluded.created_at,
			updated_at = excluded.updated_at,
			project_id = excluded.project_id,
			metadata_json = excluded.metadata_json`,
		session.ID,
		session.URL,
		session.Status,
		session.CreatedAt,
		emptyToNil(session.UpdatedAt),
		emptyToNil(session.ProjectID),
		emptyToNil(metadataJSON),
	)
	return err
}

func (b *sqliteBackend) UpsertAnnotation(annotation Annotation) error {
	payload, err := json.Marshal(annotation)
	if err != nil {
		return err
	}

	_, err = b.db.Exec(
		`INSERT INTO annotations (id, session_id, data_json, updated_at)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			session_id = excluded.session_id,
			data_json = excluded.data_json,
			updated_at = excluded.updated_at`,
		annotation.ID,
		annotation.SessionID,
		string(payload),
		emptyToNil(annotation.UpdatedAt),
	)
	return err
}

func (b *sqliteBackend) DeleteAnnotation(annotationID string) error {
	_, err := b.db.Exec(`DELETE FROM annotations WHERE id = ?`, annotationID)
	return err
}

func (b *sqliteBackend) InsertEvent(event Event) error {
	if event.Sequence <= 0 {
		return errors.New("event sequence must be greater than zero")
	}

	payload, err := json.Marshal(event)
	if err != nil {
		return err
	}

	_, err = b.db.Exec(`INSERT OR REPLACE INTO events (sequence, session_id, data_json) VALUES (?, ?, ?)`, event.Sequence, event.SessionID, string(payload))
	return err
}

func (b *sqliteBackend) Close() error {
	if b == nil || b.db == nil {
		return nil
	}
	return b.db.Close()
}

func emptyToNil(value string) any {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return value
}
