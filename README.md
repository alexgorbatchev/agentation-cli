# agentation

Go-based CLI companion for the Agentation HTTP server.

## Build

```bash
cd cli
go build ./cmd/agentation
```

Or with just from this directory:

```bash
cd cli
just build
```

## Usage

```bash
agentation [--base-url http://localhost:4747] <command>
```

Commands:

- `sessions`
- `session <session-id>`
- `pending [--session <id>]`
- `ack <annotation-id>`
- `resolve <annotation-id> [--summary "..."]`
- `dismiss <annotation-id> --reason "..."`
- `reply <annotation-id> --message "..."`
- `watch [--session <id>] [--batch-window 10] [--timeout 120]`
- `server <serve|start|stop|status>`

Add `--json` to any command for machine-readable output.

## Server process management

```bash
# Run in foreground
agentation server serve --address 127.0.0.1:4747

# Run in background
agentation server start --background

# Run attached to current shell
agentation server start --foreground

agentation server status
agentation server stop
```

## Environment variables

- `AGENTATION_HTTP_URL` (default: `http://localhost:4747`)
- `AGENTATION_STORE` (`sqlite` by default, set to `memory` for in-memory mode)
- `AGENTATION_DB_PATH` (explicit SQLite DB path override)
- `XDG_DATA_HOME` (used for default SQLite location when `AGENTATION_DB_PATH` is unset)
- `AGENTATION_SERVER_PID_FILE` (override PID file for `server start/stop/status`)
- `AGENTATION_SERVER_LOG_FILE` (override log file for background server mode)

## SQLite storage location

By default, data is stored in SQLite at:

- `$XDG_DATA_HOME/agentation/store.db` (if `XDG_DATA_HOME` is set)
- otherwise `~/.local/share/agentation/store.db`

You can override the DB file completely with:

```bash
AGENTATION_DB_PATH=/absolute/path/store.db agentation server serve
```
