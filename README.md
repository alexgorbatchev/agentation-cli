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
agentation <command>
```

Commands:

- `ack <annotation-id> [--base-url <url>] [--json]`
- `dismiss <annotation-id> [--base-url <url>] --reason "..." [--json]`
- `generate --fix-loop-skill`
- `pending <project-id> [--base-url <url>] [--json]`
- `project <project-id> [--base-url <url>] [--json]`
- `projects [--base-url <url>]` (project IDs active in the last 24h)
- `reply <annotation-id> [--base-url <url>] --message "..." [--json]`
- `resolve <annotation-id> [--base-url <url>] [--summary "..."] [--json]`
- `start [--server-addr host:port|0] [--router-addr host:port|0] [--foreground|--background]`
- `status`
- `stop`
- `watch <project-id> [--base-url <url>] [--batch-window 10] [--timeout 300] [--json]`

Add `--json` to API/data commands for machine-readable output.

You can set a default API endpoint with:

```bash
AGENTATION_BASE_URL=http://127.0.0.1:4747 agentation pending project-alpha --json
```

## Project-scoped filtering

Project scoping is required for `pending` and `watch`.
Use `projects` to discover project IDs with activity in the last 24 hours.

```bash
agentation projects --json
agentation project project-alpha --json
agentation pending project-alpha --json
agentation watch project-alpha --timeout 300 --batch-window 10 --json
```

## Router token auth (`AGENTATION_ROUTER_TOKEN`)

When `AGENTATION_ROUTER_TOKEN` is set, router requests that can mutate session state or trigger editor side effects require auth:

- `POST /register`
- `POST /unregister`
- `GET|POST /open`

Provide the token using either:

- `X-Agentation-Token: <token>`
- `Authorization: Bearer <token>`

`/ping` remains unauthenticated for liveness/session resolution checks.

## SSE delivery semantics (`watch` / `/events`)

`agentation watch` first drains `/pending`, then listens on SSE (`/events?agent=true` or `/sessions/{id}/events?agent=true`).

Operational guarantees/limits:

- Events include a monotonically increasing sequence ID (`id` in SSE frames).
- Server keepalives are emitted as SSE comments (`: ping`) every ~30s.
- Delivery uses explicit backpressure semantics to avoid silent event drops under load.
- Trade-off: a consistently slow consumer can increase end-to-end latency while pressure is applied.
- `/pending` remains the source of truth for reconciliation if a stream disconnects.

## Skill generation helpers

```bash
agentation generate --fix-loop-skill
```

This prints the embedded Agentation fix-loop skill markdown from the CLI binary.

## Lifecycle management

```bash
# Start both services (default)
agentation start

# Start with explicit addresses
agentation start --server-addr 127.0.0.1:4747 --router-addr 127.0.0.1:8787

# Disable one service by setting address to 0
AGENTATION_SERVER_ADDR=0 agentation start
AGENTATION_ROUTER_ADDR=0 agentation start

agentation status
agentation stop
```

Notes:

- `start` runs as a **single PID** that manages both server and router.
- By default, both services start.
- Set `AGENTATION_SERVER_ADDR=0` to disable server, or `AGENTATION_ROUTER_ADDR=0` to disable router.
- `--server-addr` / `--router-addr` override environment values.
- `--foreground` runs in current shell; `--background` daemonizes.

## Environment variables

- `AGENTATION_BASE_URL` (default base URL for API commands: `http://localhost:4747`)
- `AGENTATION_STORE` (`sqlite` by default, set to `memory` for in-memory mode)
- `AGENTATION_DB_PATH` (explicit SQLite DB path override)
- `XDG_DATA_HOME` (used for default SQLite location when `AGENTATION_DB_PATH` is unset)
- `AGENTATION_SERVER_ADDR` (default server address for `agentation start`; use `0` to disable)
- `AGENTATION_ROUTER_ADDR` (default router address for `agentation start`; use `0` to disable)
- `AGENTATION_PID_FILE` (override single PID file for stack lifecycle)
- `AGENTATION_LOG_FILE` (override stack supervisor log file for background mode)
- `AGENTATION_SERVER_LOG_FILE` (override server log file)
- `AGENTATION_ROUTER_LOG_FILE` (override router log file)
- `AGENTATION_ROUTER_ADDRESS` (legacy fallback router address if `AGENTATION_ROUTER_ADDR` is unset)
- `AGENTATION_ROUTER_TOKEN` (optional auth token; when set, required by `/register`, `/unregister`, and `/open`)
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
AGENTATION_DB_PATH=/absolute/path/store.db agentation start --foreground
```
