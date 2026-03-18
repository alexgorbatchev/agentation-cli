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
- `start [--server] [--server-addr host:port] [--router] [--router-addr host:port] [--foreground|--background]`
- `stop [--server] [--router]`
- `status [--server] [--router]`

Add `--json` to API/data commands for machine-readable output.

## Lifecycle management

```bash
# Start server only (default behavior)
agentation start

# Start server + router
agentation start --server --router

# Start server + router using explicit addresses
agentation start --server --server-addr 127.0.0.1:4747 --router --router-addr 127.0.0.1:8787

# Start only router
agentation start --router

agentation status
agentation stop
```

Notes:

- If neither `--server` nor `--router` is provided, `start` launches only the server.
- `AGENTATION_SERVER_ADDR` sets the default server listen address for `start`.
- If `AGENTATION_ROUTER_ADDR` is set, `start` also launches the router by default.
- `--foreground` supports one selected service at a time; use `--background` for multi-service startup.
- Router lifecycle is built into the same `agentation` binary (no external router binary required).

## Environment variables

- `AGENTATION_HTTP_URL` (default: `http://localhost:4747`)
- `AGENTATION_STORE` (`sqlite` by default, set to `memory` for in-memory mode)
- `AGENTATION_DB_PATH` (explicit SQLite DB path override)
- `XDG_DATA_HOME` (used for default SQLite location when `AGENTATION_DB_PATH` is unset)
- `AGENTATION_SERVER_ADDR` (default server address for `agentation start` when `--server-addr` is not set)
- `AGENTATION_SERVER_PID_FILE` (override PID file for server lifecycle)
- `AGENTATION_SERVER_LOG_FILE` (override log file for server background mode)
- `AGENTATION_ROUTER_ADDR` (default router address and auto-enable switch for `agentation start`)
- `AGENTATION_ROUTER_PID_FILE` (override PID file for router lifecycle)
- `AGENTATION_ROUTER_LOG_FILE` (override log file for router background mode)
- `AGENTATION_ROUTER_ADDRESS` (router serve listen address default)
- `AGENTATION_ROUTER_TOKEN` (optional auth token for mutating router endpoints)
- `AGENTATION_ROUTER_BODY_LIMIT` (max router request body size)
- `AGENTATION_ROUTER_FORWARD_TIMEOUT` (router forward timeout)
- `AGENTATION_ROUTER_READ_TIMEOUT` / `AGENTATION_ROUTER_WRITE_TIMEOUT`
- `AGENTATION_ROUTER_READ_HEADER_TIMEOUT` / `AGENTATION_ROUTER_IDLE_TIMEOUT`
- `AGENTATION_ROUTER_SESSION_STALE_AFTER`
- `AGENTATION_ROUTER_ALLOW_ABSOLUTE_PATHS`
- `AGENTATION_ROUTER_ENFORCE_ROOT_BOUNDS`

## SQLite storage location

By default, data is stored in SQLite at:

- `$XDG_DATA_HOME/agentation/store.db` (if `XDG_DATA_HOME` is set)
- otherwise `~/.local/share/agentation/store.db`

You can override the DB file completely with:

```bash
AGENTATION_DB_PATH=/absolute/path/store.db agentation start --foreground --server
```
