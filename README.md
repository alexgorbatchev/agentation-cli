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

Add `--json` to any command for machine-readable output.

## Environment variables

- `AGENTATION_HTTP_URL` (default: `http://localhost:4747`)
