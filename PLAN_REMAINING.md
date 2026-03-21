# Estuary: Remaining Work

This document tracks what is not yet done relative to `plan.md`.
Cross-reference `FEATURES.md` for the current shipped-feature index.

---

## Status Snapshot

| Milestone | Status |
|---|---|
| 1 — Core Shell | ✅ complete |
| 2 — Session Runtime | ⚠ partial |
| 3 — Persistence and Resume | ⚠ partial |
| 4 — Migration | ❌ not started |
| 5 — Traits | ❌ not started |
| 6 — Boundaries and Habitat Settings Polish | ❌ not started |
| 7 — Daily-Use Polish | ❌ not started |

---

## Milestone 2 Remainder — Session Runtime

### Streaming output into TUI

**What exists:** Turn execution runs `claude -p` / `codex exec` with `CombinedOutput` — the process blocks, output is parsed after exit, and the full assistant response appears at once.

**What is needed:**
- Replace blocking `cmd.CombinedOutput()` in `internal/habitats/runtime.go` with `cmd.StdoutPipe()` + line-by-line JSON read loop.
- Emit a Bubble Tea `Cmd` that streams events back via a channel or recursive `tea.Cmd` chain.
- Surface incremental text deltas in the chat timeline as they arrive.
- The `TurnResult` struct or a new `TurnEvent` type carries both delta and final-state variants.

**Files to touch:** `internal/habitats/runtime.go`, `internal/app/model.go` (new message types for delta vs. complete).

---

### Model discovery from installed CLIs

**What exists:** `HabitatHealth.AvailableModels` is scaffolded as `[]string` and currently left empty. The new-session flow still uses free-text model entry.

**What is needed:**
- `ProbeHabitat` in `internal/prereq/probe.go` runs the best stable native model-list command available and parses available model IDs into `health.AvailableModels`.
- Same for Codex, using the best stable native model-list command available there.
- Model picker in the new-session form switches from free-text entry to a list drawn from probed results, falling back to free-text when probe is empty.
- `ModelDescriptor` (already in `domain/types.go`) gets populated and stored.

**Files to touch:** `internal/prereq/probe.go`, `internal/app/model.go` (new-session field → list picker).

---

### Authentication state probing

**What exists:** `HabitatHealth.Authenticated` is always `false`; a placeholder warning is always emitted.

**What is needed:**
- Prefer CLI-visible readiness/auth signals over hardcoded filesystem assumptions when possible.
- Claude: use a stable command/output signal to determine whether the CLI is ready to execute authenticated turns.
- Codex: do the same using the native CLI's ready/authenticated state.
- Populate `health.Authenticated` and remove the placeholder warning once probed.
- Ecosystem screen shows `authenticated` / `not authenticated` accurately.

---

### Habitat package split

**What the plan specifies:** `internal/habitats/claude` and `internal/habitats/codex` as separate packages with habitat-specific runtime, migration, and boundary projection logic.

**What exists:** A single `internal/habitats/runtime.go` with `runClaude` and `runCodex` as private methods on one shared `Runtime` struct.

**Priority note:** This is a worthwhile refactor, but it is not the highest-value blocker. Streaming, auth/model discovery, and runtime event logging should land first.

**What is needed:**
- Create `internal/habitats/claude/runtime.go` with a `Runtime` struct that owns Claude-specific execution and output parsing.
- Create `internal/habitats/codex/runtime.go` with the same for Codex.
- The parent `internal/habitats` package retains the registry and routing (`Registry()`, `HabitatForModel()`).
- `internal/habitats/runtime.go` becomes a thin dispatcher that delegates to the correct sub-package runtime.
- This split is the foundation for Milestone 4 (habitat-specific migration logic) and Milestone 5 (habitat-specific trait projection).

---

## Milestone 3 Remainder — Persistence and Resume

### Session resume on open

**What exists:** Sessions and native session IDs are persisted. When a session is selected, its transcript is loaded from the DB. Provider-backed turns already reuse the stored native session ID where supported, but there is no explicit user-facing "resume" action or visible resume state.

**What is needed:**
- When a session with a non-empty `NativeSessionID` is selected, expose a `Resume` action in the UI.
- On resume, `chat.Service.Send` re-uses the stored `--resume` / `exec resume` path.
- On first message after restart (no resume attempt yet), the habitat runtime automatically uses `NativeSessionID` if present.
- This is already partially wired, but there is no explicit user-facing resume flow or status indication.

### Graceful degraded restore

**What is needed:**
- If `NativeSessionID` is stale or the habitat rejects resume, fall back to transcript-only mode.
- Emit a system message in the chat timeline noting that the native session could not be resumed and the transcript was restored.
- Record a `session.resumed` or `session.restore-failed` runtime event.

### Runtime event backfill

**What exists:** Only `session.created` is written as an event. The schema supports the full event vocabulary from `plan.md`.

**What is needed:** Write runtime events for:
- `session.resumed`
- `turn.started` / `turn.completed`
- `assistant.delta` (per streaming chunk, once streaming is implemented)
- `tool.started` / `tool.output` / `tool.finished`
- `habitat.error`
- `session.closed`

These are needed before Milestone 4 because migration checkpoints read from the event log.

---

## Milestone 4 — Migration

### Overview

Migration lets a user change the model (and therefore habitat) inside one Estuary session. The session timeline is continuous; the habitat switch is recorded as a checkpoint event.

### Migration checkpoint service

Create `internal/migration` package with a `Service` that:
- Reads the current session's message transcript and recent runtime events.
- Compresses them into a `MigrationCheckpoint` struct (already in the DB schema).
- Checkpoint content per `plan.md`:
  - active objective
  - important prior decisions
  - current folder path / model / habitat
  - rolling conversation summary
  - open tasks/questions
  - active traits in effect
  - recent notable tool outputs
  - habitat-specific notes

### Model/habitat switch action

- Add a key binding (e.g. `m`) to open a "Change Model" modal.
- Modal lists probed available models (from Milestone 2 model discovery).
- On selection:
  1. Create a migration checkpoint for the current session state.
  2. Update `session.CurrentModel` and `session.CurrentHabitat`.
  3. Clear `session.NativeSessionID` (new habitat session needed).
  4. Persist updated session to DB.
  5. Record `model.changed` and `habitat.changed` runtime events.

### Continuation prompt injection

- After a habitat switch, the first turn sent to the new habitat includes a minimal continuation context derived from the migration checkpoint.
- This is the only text Estuary injects — no hidden system prompt, no persona.
- The injection is explicit and visible as a `system` role message in the timeline.

### Timeline notices

- Migration events appear as visual dividers in the chat timeline (e.g., `── migrated to claude-opus-4 ──`).
- Checkpoint creation is shown as a `summary` role message with truncated content.

---

## Milestone 5 — Traits

### Overview

Traits are commands, skills, and tools defined once in Estuary and projected into compatible habitats. The DB schema (`traits` table) already exists.

### Trait service

Create `internal/traits` package with a `Service` that:
- CRUD for traits in SQLite.
- Returns trait list with compatibility metadata per habitat.
- Resolves which traits are active for a given session.

### Trait types and projection

Three categories per `plan.md`:

**Commands** — executable or provider-routed actions. For Claude: slash commands injected via the prompt or `--append-system-prompt`. For Codex: TBD based on available injection path.

**Skills** — instruction bundles or role overlays. Projected as user-turn injections or system-level context at session start.

**Tools** — capability definitions. Map to habitat-native tool config where available; fall back to injected instructions or `unsupported` state.

Each trait must store `dispatch_mode`: `provider-native | injected-context | unsupported`.

### Traits UI

- Traits setup screen accessible via key binding (e.g. `T`).
- Lists all defined traits with compatibility labels: `Shared`, `Claude-only`, `Codex-only`, `Partial`.
- Create/edit/delete trait flow (modal form).
- Active traits for the current session shown in the right context pane (replaces the current "Traits UI is planned for a later milestone." placeholder).

### Session-level invocation

- Active traits are applied at session start (for bootstrap-mode traits).
- Command-type traits can be invoked from the composer via a prefix (e.g., `/trait-name`).
- If a trait is unsupported by the current habitat, the user sees an explicit error rather than silent failure.

---

## Milestone 6 — Boundaries and Habitat Settings Polish

### Boundary profile management UI

**What exists:** Four hardcoded canonical profiles rendered in a read-only list. There is no ability to create, edit, or set a default.

**What is needed:**
- Per-session boundary profile can be changed after session creation (triggers a habitat settings update, not a full migration).
- `Exact / Approximated / Unsupported` compatibility indicator shown prominently when a profile is selected.
- `Full Access` profile is visually flagged as unsafe at every point of selection (not just in the description text).
- Optional: allow user-defined custom profiles stored in the DB.

### Habitat Settings screen

Replace the current `Ecosystem` screen or add a separate `Habitat Settings` view that shows per-habitat:
- installed / not installed
- authenticated / not authenticated
- available models (from probe)
- config file path (e.g. `~/.claude`, `~/.codex`)
- key settings Estuary can surface safely (e.g. Claude permission mode default, Codex sandbox default)
- default boundary mapping behavior
- last probe timestamp

This maps to the planned `internal/habitatconfig` package. Currently these responsibilities are split across `internal/prereq` and `internal/store`. The refactor:
- `internal/prereq` stays as health probing (executables, version, auth).
- `internal/habitatconfig` owns the settings read/write layer and the per-habitat setup view contract.

### First-run setup flow

- On first launch (no sessions, fresh DB), show a guided onboarding: ecosystem probe → confirm habitats → select default boundary profile → create first session.
- This is a modal or wizard layered over the normal layout, not a separate binary mode.

---

## Milestone 7 — Daily-Use Polish

### Multi-session tabs

**What exists:** Sessions are listed in the left pane and can be navigated with `j`/`k`. There is no tab bar.

**What is needed:**
- Render a tab bar above the center pane showing open session titles.
- Tab switching via number keys or `[` / `]`.
- Visual indicator for sessions with unread turns or active status.
- Closing a tab does not delete the session; it removes it from the open-tab set.

### Search and filter

- Filter the session list by title, folder path, model, or habitat.
- Accessible via `/` in the sessions pane.
- Clears on `esc`.

### Better error states

- Habitat execution errors (e.g. `claude` exits non-zero) surfaced as a dismissible inline error block in the chat timeline, not just a status bar message.
- Persistent error state on the session tile in the left pane.
- Clear recovery action offered (e.g. `r` to retry, `m` to migrate to another model).

### Diagnostics / log view

- Key binding (e.g. `d`) opens a diagnostics pane showing recent runtime events for the selected session.
- Useful for debugging habitat errors, migration steps, and boundary translation.

---

## Priority Order

If work resumes now, the highest-value order is:

1. Streaming output into the TUI plus runtime event logging.
2. Authentication probing and model discovery.
3. Resume UX and graceful degraded restore.
4. Habitat package split into `claude` and `codex`.
5. Migration.
6. Traits.
7. Daily-use polish.

### Help screen

The current help screen is a static key list. Improve it:
- Group bindings by context (global, session pane, chat, composer, modals).
- Show the current boundary profile and habitat for context.
- Link to `FEATURES.md` or show a version string.

---

## Architectural Gaps Not Tied to a Milestone

### `ctx` in Model struct

`internal/app/model.go` stores a `context.Context` as a struct field — explicitly discouraged by the Go standard library docs. Every async `tea.Cmd` captures `m.ctx` directly. The Bubble Tea pattern is to pass context from `Init` through the command closure or store only a cancel function. This should be resolved before the codebase grows further and cancellation/deadline propagation becomes load-bearing.

### Blocking turn execution UX

While the habitat process runs, the TUI is visually frozen — no spinner, no "running..." indicator, no way to cancel. Even before full streaming is implemented, a `SessionStatusActive` indicator in the chat pane and a cancel keybinding (`ctrl+c` within the composer) would improve the experience significantly.

### `ecosystem_snapshots` table growth

The table is a pure append log with no pruning. Each probe cycle adds rows. A retention policy (e.g. keep the 10 most recent rows per habitat) should be added to `UpsertEcosystemSnapshot` or as a periodic cleanup step.
