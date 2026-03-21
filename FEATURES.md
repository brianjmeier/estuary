# Estuary Features

This file is the canonical index of shipped behavior in this repository.

## Application Shell

- `implemented` Bubble Tea shell centered on a single main transcript surface instead of a permanent multi-pane dashboard.
- `implemented` Compact header and help strip showing the current directory, active model/habitat, and the most useful keybinds.
- `implemented` Minimal default keybind set focused on terminal-native use: `Enter`, `Ctrl+K`, `Esc`, and `Ctrl+C`.
- `implemented` Light and dark semantic theme token sets shared through one design system.

## Startup And Sessions

- `implemented` Estuary starts a fresh session automatically in the current working directory on launch.
- `implemented` Default startup model is `claude-sonnet-4-6`, mapped automatically to the Claude habitat.
- `implemented` Session metadata is persisted for folder, model, habitat, boundary profile, status, migration generation, native session id, and resolved boundary settings.
- `implemented` Session activity state is tracked separately from transcript history so historical sessions do not count as active work.
- `implemented` Same-folder warnings are reserved for crowded active work only: a new session warns only when two or more active sessions already target that directory.
- `implemented` `Ctrl+K` command palette lists commands and persisted sessions for switching into older work explicitly.

## Chat

- `implemented` Common chat timeline for user, assistant, system, tool, and migration-summary messages.
- `implemented` Persisted user and assistant turns in SQLite.
- `implemented` Provider-backed turn execution through streaming `claude -p` and `codex exec` runtimes.
- `implemented` Incremental assistant deltas rendered live in the transcript while a turn is still running.
- `implemented` Tool and runtime notices surfaced in the timeline during streaming turns.
- `implemented` Embedded composer in the main screen, always ready for typing at launch.
- `implemented` Composer growth behavior that expands with wrapped text up to eight visible lines before showing only the tail of the input.
- `implemented` Graceful degraded restore when provider-native resume is rejected, with transcript-only continuation and a visible system notice.

## Command Palette And Settings

- `implemented` `Ctrl+K` command palette for new session creation, session switching, model changes, boundary changes, traits, help, theme toggle, and habitat re-probing.
- `implemented` Separate settings/help surfaces instead of putting configuration controls into the main transcript layout.
- `partial` Session resume is explicit through the management surfaces rather than automatic at startup, but it is still represented as session switching rather than a richer dedicated resume workflow.

## Ecosystem And Habitat Settings

- `implemented` Habitat probing for Claude and Codex install state, version, authentication state, config hints, boundary behavior, and discovered model lists.
- `implemented` Model discovery populates migration/model controls from installed CLI help/config probes, with manual fallback.

## Boundaries

- `implemented` Canonical boundary profiles: Ask Always, Workspace Write, Read Only, and Full Access.
- `implemented` Boundary resolution metadata structure with exact, approximated, and unsupported compatibility states.
- `implemented` Habitat-native translation for Claude permission mode and Codex approval/sandbox startup settings.
- `implemented` Boundary changes happen in a dedicated control surface instead of the default chat view.

## Migration

- `implemented` Change-model flow that creates a migration checkpoint, updates the selected session’s model and habitat, clears the native session id, and logs migration events.
- `implemented` Continuation-context injection on the first post-migration turn, visible in the transcript as a system message instead of a hidden prompt.
- `implemented` Migration checkpoint summaries surfaced in the chat timeline as summary-role messages.

## Traits

- `implemented` SQLite-backed Traits registry for shared command, skill, and tool definitions.
- `implemented` Trait editor modal for creating shared traits with habitat compatibility metadata.
- `implemented` Command-trait invocation from the composer using `/trait-name`, with explicit unsupported-habitat errors instead of silent failure.

## Persistence

- `implemented` SQLite store bootstrap under `~/.estuary/data/estuary.db`.
- `implemented` Initial schema creation for sessions, session runtime, messages, events, migration checkpoints, traits, habitat settings, ecosystem snapshots, boundary profiles, session boundary resolutions, and app settings.
- `implemented` Session runtime state persistence for active-session tracking, resume intent, and pending migration continuation context.
- `partial` App settings persistence currently covers theme selection.
- `implemented` Runtime event logging for session resume, turn start/completion, assistant deltas, tool lifecycle events, habitat errors, model changes, habitat changes, boundary changes, restore failures, and session close.

## Documentation And Tooling

- `implemented` `README.md` links to this file as the authoritative current feature index.
- `implemented` Nix-based development workflow remains documented as the primary project-local toolchain path.
- `implemented` Focused unit tests cover boundary profiles, habitat mapping heuristics, split habitat runtime parsing helpers, theme token completeness, session creation, and documentation linkage.
