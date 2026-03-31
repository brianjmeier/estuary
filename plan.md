# Estuary: Native Claude Code and Codex Terminal Orchestrator

## Summary

Estuary is a terminal-first TUI shell that embeds native Claude Code and Codex sessions.

The product is:

- a native session host for Claude Code and Codex
- a model and provider switcher
- a continuity manager for switching across models and providers
- a config authority that imports, unifies, and syncs Claude/Codex setup
- a shared command sync tool that writes command files into provider-native command folders

The primary value proposition:

- open the right native tool from one place
- switch cleanly without losing working context
- manage shared setup once
- keep provider-native command systems aligned from one Estuary-owned source of truth

## Product Decisions Locked

- App name: `Estuary`
- Language/runtime: Go
- Shell: TUI (Bubble Tea)
- Main screen: single embedded native terminal via raw PTY passthrough
- Estuary reserves terminal rows for always-visible chrome (header + footer)
- Session model: multiple persisted sessions
- Session continuity: restart + handoff injection (no PTY reattachment)
- Session restore: relaunch native tool with resume flag; fall back to handoff if unavailable
- Model catalog: Estuary-owned registry with probe validation
- Shared config: Estuary imports provider configs, resolves conflicts once, becomes source of truth, syncs on startup
- Config scope: unify commands, bash permissions, and skills; provider-specific knobs live in a provider-scoped section of the Estuary config file
- Shared commands: stored as individual files under one `~/.config/estuary/commands/` directory; provider routing via per-file metadata; synced into provider-native command folders on startup
- Command invocation happens inside Claude Code or Codex native UX only; Estuary does not expose a command invocation UI
- Command bodies are raw pass-through text; Estuary does not implement templating or argument preprocessing
- Provider-scoped commands are allowed in the shared command directory
- Drift detection is per command file and per provider config source
- Traits (SQLite) are replaced by filesystem command files; no retrocompatibility required
- HandoffPacket extends MigrationCheckpoint (same type, extended fields)
- `model.go` rewrite is additive: new PTY-first model written alongside old code, old code removed after new works
- Development environment: Nix flakes
- Every implemented feature must update `FEATURES.md`
- `README.md` must reference `FEATURES.md` as the authoritative feature index

## Naming System

### Primary Product Vocabulary

- App: `Estuary`
- Sessions: `Sessions`
- Providers/backends: `Providers` (replacing `Habitats` in new code)
- Models: `Models`
- Shared setup: `Commands` (filesystem-first, replacing `Traits`)
- Continuity switch: `Handoff`
- Health: `Ecosystem`
- Permissions: `Boundaries`

### Legacy Names (kept in old code, not used in new code)

- `Habitats` → replaced by `Providers`
- `Traits` → replaced by `Commands`
- `Migration` → replaced by `Handoff`

## Design Language

### Theme Statement

Estuary should feel like a coastal control room for intelligent systems: calm, literate, habitat-aware, and operationally trustworthy.

### Visual Principles

- Different providers, one shoreline
- Technical clarity first, metaphor second
- Strong hierarchy with generous breathing room
- Calm operational feel, not flashy AI tool theatrics
- Shared product identity across all providers
- Restraint in color, motion, and ornament

### Chrome Layout

Estuary reserves terminal rows around the native PTY session:

- Header (2 rows): session name, current directory, active model, active provider, boundary profile, native runtime state, config sync state
- Footer (1 row): minimal keybind hints

The PTY fills all remaining rows.

### Color Semantics

- water: focus, active selection, handoff flow
- reed: safe progress, confirmation, ready states
- clay: warmth, provider annotation, grounded emphasis
- amber: warning, approximation, caution
- rust: danger, destructive actions, unsafe full access

Provider tint:
- Claude: warm mineral / clay / amber
- Codex: cool water / steel-blue

### Theme Token Requirements

Semantic tokens (same as before, unchanged):
- `bg.canvas`, `bg.surface`, `bg.panel`
- `fg.primary`, `fg.muted`
- `border.soft`
- `accent.water`, `accent.reed`, `accent.clay`
- `status.warning`, `status.danger`, `status.success`
- `habitat.claude`, `habitat.codex`

Both light and dark themes derive from these tokens.

## Architecture

### Top-Level Components

1. `cmd/estuary` — app entrypoint
2. `internal/app` — root Bubble Tea model (PTY-first rewrite)
3. `internal/pty` — PTY manager, terminal embedding, resize, process lifecycle
4. `internal/providers` — provider terminal adapters (Claude, Codex)
5. `internal/sessions` — session lifecycle, persistence, restore orchestration
6. `internal/handoff` — handoff packet generation, storage, injection
7. `internal/configsync` — provider config detection, import, conflict resolution, startup sync, drift detection
8. `internal/commands` — command file parser/validator, provider sync writers, import from provider-native folders
9. `internal/boundaries` — boundary profile model, provider-native translation
10. `internal/store` — SQLite persistence
11. `internal/prereq` — provider probing, install/auth/model detection

### Legacy Components (kept during transition, removed in Phase 8)

- `internal/chat` — unified chat runtime (legacy)
- `internal/habitats` — habitat turn streaming (legacy)
- `internal/traits` — SQLite trait registry (legacy, replaced by `internal/commands`)
- `internal/migration` — migration checkpoint service (extended into `internal/handoff`)

## Core Runtime Model

### Session

A session binds:
- one folder path
- one active model and provider selection
- one boundary profile
- one native terminal runtime record
- one latest handoff packet reference
- one config sync status snapshot
- native session identifiers if resumable

### Native Terminal Runtime

A runtime record tracks:
- provider (claude | codex)
- process PID
- PTY state
- startup args and env
- attach strategy (resume | handoff | fresh)
- last exit info

### HandoffPacket (extends MigrationCheckpoint)

Contains:
- active objective
- concise recent work summary
- open tasks
- important decisions
- relevant file and directory references
- source model and provider
- target model and provider (when switching)
- boundary profile
- timestamp
- optional user note

Used for:
- cross-provider switching
- same-provider restart fallback
- failed native resume
- reopening sessions without viable native attachment
- migrating old unified-chat sessions into native sessions

## PTY Runtime Architecture

### Raw Terminal Passthrough

Estuary uses raw PTY passthrough: the native tool process gets a real PTY and its input/output flows directly through the system terminal. Estuary does not re-render the terminal content.

Estuary claims:
- top 2 rows: header chrome
- bottom 1 row: footer chrome

The native tool occupies all remaining rows. Resize events are forwarded to the PTY process.

### Provider Terminal Adapters

Each provider adapter defines:
- startup command and args
- env vars
- cwd behavior
- boundary projection to CLI flags
- config sync target paths
- native session ID extraction (if available)
- same-provider switch strategy
- handoff injection method
- resume capability rules

Files:
- `internal/providers/claude_terminal.go`
- `internal/providers/codex_terminal.go`

## Continuity and Switching

### Session Restore

When a session is reopened:
1. Estuary tries native resume (Claude `--resume <id>`, Codex thread resume)
2. If native resume is unavailable or fails, inject latest handoff packet as startup context
3. Persist new runtime metadata

No PTY reattachment. The user manages their own shell persistence if they want it.

### Same-Provider Switch

Use the provider's best native strategy:
- if in-place switching exists (e.g., Claude model flag), use it
- otherwise restart the same provider and inject handoff

Do not ask the user to choose the switch mechanism.

### Cross-Provider Switch

Always:
1. Generate handoff packet
2. Stop or detach old native session
3. Start target provider natively with selected model and boundaries
4. Inject handoff into target session startup
5. Persist new runtime metadata
6. Record switch event

## Config and Sync

### Estuary Canonical Config

Location: `~/.config/estuary/config.yaml`

Contains:
- theme and app settings
- model registry settings
- provider paths and detection overrides
- sync state metadata
- boundary defaults
- provider-specific override sections (e.g., `claude:`, `codex:`)
- onboarding and import metadata

Shared across providers (unified):
- commands (managed via `~/.config/estuary/commands/`)
- bash permissions
- skills

Provider-specific (in provider section):
- any setting that only makes sense for one provider

### Config Import / Conflict Resolution / Sync

`internal/configsync` responsibilities:
- detect Claude and Codex config sources
- import existing provider config
- map overlaps into Estuary canonical config
- surface conflicts
- write Estuary config
- sync provider-native config on startup
- detect drift
- pause and ask on conflicts or drift (never silently overwrite)

### Sync Precedence

1. Estuary canonical config
2. Provider-specific override sections inside Estuary config
3. Imported provider-native config (only during onboarding or re-import)
4. Probe results (only for validation)

### Drift Policy

When provider-native config changes outside Estuary:
- detect drift
- ask whether to keep Estuary as source of truth or re-import provider changes
- never silently clobber external changes after drift is detected

## Shared Commands

### Source of Truth

Commands live as individual files:

```text
~/.config/estuary/commands/
```

One file = one canonical command.

### File Format

```markdown
---
name: plan-work
description: Turn a rough task into an implementation plan
providers: [claude, codex]
---
Analyze the current repository state and produce a concrete implementation plan.
```

Provider-scoped example:

```markdown
---
name: review-security
description: Security-focused review
providers: [claude]
---
Review these changes for security issues and concrete remediations.
```

### Required Frontmatter

- `name`
- `description`
- `providers` — one of: `[claude]`, `[codex]`, `[claude, codex]`

### Command Body Rules

- body is raw provider text
- Estuary does not parse arguments
- Estuary does not implement placeholders or templating
- body is synced as-is into provider-native command files

### Provider-Specific Overrides (Deferred)

Do not implement unless a real mismatch forces it:
- `claude_name`, `codex_name`
- `claude_body`, `codex_body`

Default: one file, one body, one metadata block, provider routing by metadata.

### Command Sync Behavior

During startup sync:
- read all command files
- validate metadata
- route into provider-native command folders based on `providers`
- compare per-file drift before overwrite
- pause and ask if provider file drift is detected

Estuary does not expose commands as an in-app browser or invocation UI. Users invoke them inside Claude Code or Codex natively.

## Data Model Changes

### Sessions Table

Keep `sessions`. Add or clarify:
- `runtime_kind` pointing toward native terminal
- `last_handoff_packet_id`
- `config_sync_state`
- `attach_strategy`
- `resume_strategy`
- `last_switch_type`

### New Persistence Tables

Add:
- `handoff_packets`
- `config_sync_runs`
- `config_conflicts`
- `provider_config_sources`
- `provider_command_sync_state`

### Legacy Transcript Handling

- keep old `messages` and chat state for migration compatibility
- use legacy transcript data only to generate handoffs for old sessions
- do not invest in transcript-first UX or runtime assumptions

## Implementation Sequence

### Phase 1: Spec and Docs Pivot (current)

Update `plan.md`, `README.md`, `FEATURES.md`:
- rewrite product framing around embedded native terminals
- define switching and shared config as the core value prop
- document file-per-command sync model
- mark unified chat features as legacy or slated for removal

### Phase 2: PTY Runtime Foundation

Implement:
- PTY manager (`internal/pty`)
- terminal embedding in Bubble Tea (raw passthrough with chrome rows reserved)
- provider terminal adapter interface
- startup / restart / resize / input flow
- persistence for runtime metadata

Acceptance: Estuary opens Claude or Codex in an embedded native terminal and supports real interaction.

### Phase 3: Terminal-First UI Refactor

Implement:
- terminal-first main view (additive rewrite of `model.go`)
- compact always-visible header and footer
- palette changes (`Ctrl+K` control surface)
- old composer and transcript moved to legacy path

Acceptance: Estuary is operationally usable without any unified transcript or composer UI.

### Phase 4: Session Persistence and Restore

Implement:
- native runtime session persistence
- reopen existing session
- best-effort native resume
- fallback restart with handoff injection

Acceptance: multiple persisted sessions can be reopened as native terminal sessions.

### Phase 5: Handoff and Switching

Implement:
- handoff packet generation and storage (extending MigrationCheckpoint)
- same-provider switch behavior per provider
- cross-provider switch flow
- degraded restore via handoff

Acceptance: model and provider switches preserve continuity without relying on unified transcript runtime.

### Phase 6: Config Onboarding / Import / Sync

Implement:
- provider config detection
- onboarding flow
- conflict resolution
- canonical Estuary config writing
- startup sync
- drift detection and ask-before-overwrite behavior

Acceptance: Estuary becomes the stable source of truth for shared setup.

### Phase 7: Shared Command Directory and Provider Sync

Implement:
- `~/.config/estuary/commands/` source-of-truth directory
- frontmatter parser and validator
- provider-native command folder writers
- provider command import
- per-file drift detection

Acceptance: user edits a command file once and sees it appear in supported provider-native command systems after startup sync.

### Phase 8: Legacy Cleanup

Remove or quarantine:
- unified chat runtime as default execution path
- chat composer
- slash command picker
- trait injection flow
- transcript-centered rendering

Keep only what is necessary for legacy migration and handoff generation from old sessions.

## Test Cases and Scenarios

### Terminal Runtime

- starts Claude native terminal
- starts Codex native terminal
- forwards input correctly
- handles resize correctly
- persists runtime metadata
- handles native exit and restart cleanly

### Sessions

- create multiple sessions
- switch between sessions
- reopen session with native resume when possible
- fallback to fresh start plus handoff when resume fails

### Switching

- Claude same-provider model switch
- Codex same-provider model switch
- Claude to Codex switch with handoff
- Codex to Claude switch with handoff

### Config Sync

- import Claude config only
- import Codex config only
- import both with conflicts
- provider-specific override preservation
- drift detection and explicit resolution flow

### Commands

- parse valid command file frontmatter and body
- reject invalid `providers` metadata
- sync shared command to both providers
- sync Claude-only command only to Claude
- sync Codex-only command only to Codex
- import provider-native command files into Estuary command files
- detect per-file drift without global false positives
- do not require Estuary runtime invocation support

### Legacy Migration

- old chat-backed session generates handoff
- migrated session opens natively
- old transcript remains available for migration only, not primary runtime UX

## Acceptance Criteria

The pivot is complete when:

- Estuary's main screen is an embedded native Claude/Codex terminal
- Estuary chrome (header + footer) is always visible around the PTY
- user can persist and switch multiple sessions
- model selection opens the correct provider
- same-provider switching uses provider-specific fixed behavior
- cross-provider switching uses structured handoff
- Estuary imports and owns shared config
- Estuary syncs provider-native config on startup
- Estuary uses `~/.config/estuary/commands/` as a file-per-command source of truth
- command files sync into provider-native command folders based on provider metadata
- drift is detected and resolved explicitly
- unified chat is no longer the main product contract
- docs and feature inventory match the new product truth

## Development Environment

Nix flakes remain the canonical development environment definition.

```bash
source /nix/var/nix/profiles/default/etc/profile.d/nix-daemon.sh
nix develop
```

Then inside the shell:

```bash
go test ./...
go run ./cmd/estuary
golangci-lint run
```

## Assumptions and Defaults

- PTY embedding is feasible in the current Go TUI stack.
- Provider-native command systems are file/folder based or can be adapted through file sync.
- Command metadata remains intentionally minimal: `name`, `description`, `providers`.
- Command bodies are raw pass-through text with no Estuary templating.
- Provider-scoped commands are allowed in the shared command directory.
- Provider-specific command body or name overrides are deferred until a real incompatibility requires them.
- Existing SQLite `traits` data does not need migration; the filesystem is the new source of truth.
- Old transcript data is kept only for migration and handoff generation.
- `FEATURES.md` and `README.md` must be updated alongside every shipped behavior change.
- No PTY reattachment; users manage their own shell persistence.
- Sessions are always restart-based; handoff packets carry continuity.
