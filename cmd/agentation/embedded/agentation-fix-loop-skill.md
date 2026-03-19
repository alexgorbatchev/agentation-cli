---
name: agentation-fix-loop
description: >-
  Watch for Agentation annotations and fix each one using the Agentation CLI.
  Runs `agentation watch` in a loop — acknowledges each annotation, makes the
  code fix, then resolves it. Use when the user says "watch annotations",
  "fix annotations", "annotation loop", "agentation fix loop", or wants
  autonomous processing of design feedback from the Agentation toolbar.
targets:
  - '*'
---

# Agentation Fix Loop (CLI)

Watch for annotations from the Agentation toolbar and fix each one in the codebase using the `agentation` CLI.

## CLI commands used by this skill

- `agentation start` / `agentation stop` / `agentation status`
- `agentation projects --json`
- `agentation pending <project-id> --json`
- `agentation watch <project-id> --json`
- `agentation ack <id>`
- `agentation resolve <id> --summary "..."`
- `agentation reply <id> --message "..."`
- `agentation dismiss <id> --reason "..."`

## Preflight (required)

### 1) Ensure the Agentation CLI is available

```bash
command -v agentation >/dev/null || { echo "agentation CLI not found on PATH"; exit 1; }
```

If this fails, install/build the CLI first (for this repo: `cd cli && just build`) and ensure the `agentation` binary is on `PATH`.

### 2) Check whether the Agentation stack is already running

```bash
agentation status
```

Then verify API reachability (default `http://127.0.0.1:4747`):

```bash
agentation projects --json >/dev/null
```

If not running or unreachable, **start it before doing anything else**:

```bash
agentation start --background
# or foreground during debugging
agentation start --foreground
```

Re-check after start:

```bash
agentation status
agentation projects --json >/dev/null
```

If you only want the HTTP API without router for this run:

```bash
AGENTATION_ROUTER_ADDR=0 agentation start --background
```

### 3) Determine the project ID and fetch pending work

Quickly extract project IDs from your app code:

```bash
rg -n --glob '*.{tsx,ts,jsx,js}' '<Agentation[^>]*projectId='
```

If you want to extract a literal string value quickly (when set as `projectId="..."`):

```bash
rg -o --no-filename --glob '*.{tsx,ts,jsx,js}' 'projectId="[^"]+"' \
  | head -n1 \
  | sed -E 's/projectId="([^"]+)"/\1/'
```

Then fetch the initial batch:

```bash
agentation pending <project-id> --json
```

Process that batch first, then enter watch mode.

### CLI behavior notes

- `agentation start` manages server + router under one process by default.
- One running Agentation stack is enough for multiple local projects/sessions.
- Do not start extra instances unless intentionally isolating ports/storage.

## Behavior

1. Call:

```bash
agentation watch <project-id> --timeout 300 --batch-window 10 --json
```

2. For each annotation in the returned batch:

   a. **Acknowledge**

   ```bash
   agentation ack <annotation-id>
   ```

   b. **Understand**
   - Read annotation text (`comment`)
   - Read target context (`element`, `elementPath`, `url`, `nearbyText`, `reactComponents`)
   - Map to likely source files before editing

   c. **Fix**
   - Make the code change requested by the annotation
   - Keep changes minimal and aligned with project conventions

   d. **Resolve**

   ```bash
   agentation resolve <annotation-id> --summary "<short file + change summary>"
   ```

3. After processing the batch, loop back to step 1.

4. Stop when:
   - user says stop, or
   - watch times out repeatedly with no new work.

## Rules

- Always acknowledge before starting work.
- Keep resolve summaries concise (1–2 sentences, mention file(s) + result).
- If unclear, ask via thread reply instead of guessing:

```bash
agentation reply <annotation-id> --message "I need clarification on ..."
```

- If not actionable, dismiss with reason:

```bash
agentation dismiss <annotation-id> --reason "Not actionable because ..."
```

- Process annotations in received order.
- Only resolve once the requested change is implemented.

## Required project-scoped loop

Use `<project-id>` as the first argument for all pending/watch commands:

```bash
agentation projects --json
agentation pending <project-id> --json
agentation watch <project-id> --timeout 300 --batch-window 10 --json
```

## Loop template

```text
Round 1:
  agentation pending <project-id> --json
  -> process all returned annotations

Round 2:
  agentation watch <project-id> --timeout 300 --batch-window 10 --json
  -> got 2 annotations
  -> ack #1, fix, resolve #1
  -> ack #2, reply (needs clarification)

Round 3:
  agentation watch <project-id> --timeout 300 --batch-window 10 --json
  -> got 1 annotation (clarification follow-up)
  -> ack, fix, resolve

Round 4:
  agentation watch <project-id> --timeout 300 --batch-window 10 --json
  -> timeout true, no annotations
  -> exit (or continue if user requested persistent watch mode)
```

## Troubleshooting

- `agentation pending` fails: Agentation is not running, base URL is wrong (`agentation start --background`), or `<project-id>` is missing.
- If using non-default server URL, pass `--base-url` or set `AGENTATION_BASE_URL`.
- If frontend keeps creating new sessions unexpectedly, verify localStorage/session behavior in the host app or Storybook setup.
