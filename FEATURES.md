# Estuary Features

This file is the canonical index of shipped behavior in this repository.

## Native Terminal Shell

- `implemented` PTY session manager (`internal/pty`): spawn, write input, resize (SIGWINCH), close, output streaming via channel.
- `implemented` Raw PTY passthrough wired into the terminal session runner (`internal/app/terminal_session.go`).
- `implemented` PTY resize forwarding via SIGWINCH signal handler with full-terminal ownership by the child process.
- `implemented` Host chrome abstraction (`internal/app/host_chrome.go`): tmux pane title first, OSC window-title fallback otherwise.
- `implemented` Output forwarder pause/resume with bounded buffering so leader-mode prompts do not drop PTY output by default.
- `implemented` `Ctrl+K` leader mode: help, session switch, model switch, reconnect, quit.
- `implemented` `Ctrl+C` passes through to the child PTY session during normal operation.
- `implemented` `cmd/estuary/main.go` updated to use `TerminalSession` as the primary entry point.

## Sessions

- `implemented` Smart startup: reopens the most recent session for the cwd when one exists; creates fresh only when none found.
- `implemented` Multiple persisted sessions switchable from leader-mode prompts; session list sorted by last-opened.
- `implemented` Session restore: uses `ResumeArgs` (with `NativeSessionID`) when available; falls back to `StartArgs` otherwise.
- `implemented` No PTY reattachment; sessions always restart the native process on reopen.
- `implemented` PTY exit handling: host chrome and terminal notice show exit status and reconnect guidance.
- `implemented` `pty_sessions` table: persists PID, attach strategy, native session ID, exit code per PTY spawn.
- `implemented` `TouchSession`: updates `last_opened_at` and marks session active when reopened.
- `implemented` `sessions.FindForFolder`: finds the most recent session for a directory path.
- `implemented` Session idle state saved when switching sessions or on PTY exit.
- `implemented` Session metadata persistence for folder, model, provider, boundary profile, status, and native session ID.
- `implemented` Same-folder warning when two or more active sessions target the same directory.

## Leader Controls

- `implemented` `Ctrl+K` as the primary control surface with a small global command set.
- `planned` richer control flows for new session creation, provider switching, boundaries, ecosystem health, and config sync after the terminal simplification lands.

## Provider Adapters

- `implemented` `TerminalAdapter` interface: `StartArgs`, `ResumeArgs`, `HandoffArgs` per provider.
- `implemented` Claude Code terminal adapter: `--resume <id>` and `--permission-mode` boundary projection.
- `implemented` Codex terminal adapter: `-a`, `-s`, `-C`, `--model` flags; resume via `CODEX_THREAD_ID` env var.
- `implemented` Claude habitat streaming runtime via `claude -p` (legacy path).
- `implemented` Codex habitat streaming runtime via `codex exec` (legacy path).

## Handoff and Switching

- `implemented` `HandoffPacket` domain type extending `MigrationCheckpoint`: adds `RecentWorkSummary`, `FileReferences`, `SourceModel`, `SourceProvider`, `TargetModel`, `TargetProvider`, `SwitchType`, `UserNote`.
- `implemented` `internal/handoff.Service`: generates `HandoffPacket` from live session state (delegates extraction to `migration.Service`); formats packet as injectable prompt text.
- `implemented` Same-provider model changes stay provider-native: Estuary labels them as native actions and does not type slash commands into the provider UI.
- `implemented` Cross-provider switch: updates the session, spawns a fresh PTY, and loads structured handoff context at provider startup so the user can type immediately.
- `implemented` `handoff_packets` table: persists every generated packet for debugging and future retrieval.
- `implemented` Model/session selection prompts run while PTY output is paused and buffered, preventing data loss during control flows.
- `implemented` MigrationCheckpoint: objective, decisions, conversation summary, open tasks, active traits, recent tool outputs (legacy path, being extended into HandoffPacket).
- `implemented` Continuation-context injection on first post-migration turn as a system message (legacy).

## Config Sync

- `planned` Estuary canonical config at `~/.config/estuary/config.yaml` as source of truth after onboarding.
- `planned` Shared config sections: bash permissions, skills, general settings unified across providers.
- `planned` Provider-specific override sections (`claude:`, `codex:`) within the same config file for provider-particular settings.
- `planned` Onboarding import flow: detect Claude and Codex config sources, map overlaps, surface conflicts, write Estuary config.
- `planned` Startup sync: write Estuary config values into provider-native config files.
- `planned` Drift detection: detect when provider-native config changed outside Estuary; ask before overwriting.

## Shared Commands

- `planned` `~/.config/estuary/commands/` as the file-per-command source of truth.
- `planned` Command file format: Markdown with frontmatter (`name`, `description`, `providers`).
- `planned` Provider routing by `providers` metadata: `[claude]`, `[codex]`, or `[claude, codex]`.
- `planned` Command body is raw pass-through text; no templating or argument preprocessing.
- `planned` Startup sync: route command files into provider-native command folders based on `providers`.
- `planned` Per-file drift detection before overwrite; pause and ask if drift is detected.
- `planned` Import: read provider-native command folders during onboarding and normalize into Estuary command files.

## Ecosystem

- `implemented` Habitat probing for Claude and Codex install state, authentication, config hints, boundary behavior, and discovered model lists.
- `implemented` Model discovery from installed CLI probes with manual fallback.

## Persistence

- `implemented` SQLite store at `~/.estuary/data/estuary.db`.
- `implemented` Schema for sessions, session runtime, messages, events, migration checkpoints, traits, habitat settings, ecosystem snapshots, boundary profiles, session boundary resolutions, app settings.
- `planned` New tables: `config_sync_runs`, `config_conflicts`, `provider_config_sources`, `provider_command_sync_state`.
- `implemented` Runtime event logging.
- `partial` App settings persistence covers theme selection.

## Legacy (Slated for Removal in Phase 8)

- `legacy` Unified Estuary chat transcript as the primary main screen.
- `legacy` Estuary-native chat composer.
- `legacy` Slash-command picker tied to chat input (`/trait-name` syntax).
- `legacy` Trait injection flow (SQLite-backed traits registry replaced by filesystem commands).
- `legacy` Turn-stream chat execution through `internal/chat` and `internal/habitats`.

## Documentation and Tooling

- `implemented` `README.md` links to this file as the authoritative current feature index.
- `implemented` Nix-based development workflow as the primary project-local toolchain path.
- `implemented` Focused unit tests covering boundary profiles, habitat mapping, theme token completeness, session creation, and documentation linkage.
