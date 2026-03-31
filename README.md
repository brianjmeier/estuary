# Estuary

Estuary is a terminal-first TUI shell that embeds native Claude Code and Codex sessions behind a unified session manager.

Instead of a chat layer on top of the providers, Estuary gives you:

- a raw PTY-backed native terminal for Claude Code or Codex with Estuary chrome around it
- persistent sessions you can switch between without losing context
- model and provider switching with structured handoff continuity
- a single shared config and command directory that syncs into both providers

The current implemented feature inventory lives in [FEATURES.md](./FEATURES.md). Treat that file as the authoritative shipped-feature index.

## Install

If Nix is not already available in your shell, load it first:

```bash
source /nix/var/nix/profiles/default/etc/profile.d/nix-daemon.sh
```

Then run Estuary directly from the repo:

```bash
nix run .#estuary
```

Or install it onto your profile:

```bash
nix profile install .#estuary
```

After that, launch it with:

```bash
estuary
```

Estuary requires `claude` and `codex` CLIs to be installed separately. It will probe for them on startup and surface missing or unauthenticated providers in the header.

## How It Works

When you open Estuary:

- the main screen is a full native terminal running Claude Code or Codex
- Estuary reserves the top two rows for status (session, directory, model, provider, boundary, sync state) and one bottom row for keybind hints
- `Ctrl+K` opens the command palette for session management, model switching, provider switching, boundary changes, and config sync

When you switch providers (e.g., Claude to Codex):

- Estuary generates a handoff packet from the current session context
- starts the target provider natively with that context injected
- persists the runtime metadata so you can reopen it later

When Estuary starts:

- it syncs your `~/.config/estuary/commands/` directory into provider-native command folders
- it syncs shared config (bash permissions, skills, settings) into each provider's config
- provider-specific settings stay in a provider-scoped section of `~/.config/estuary/config.yaml`

## Development

This repository uses Nix flakes for the project-local toolchain. If Nix is not already available in your shell, load it first:

```bash
source /nix/var/nix/profiles/default/etc/profile.d/nix-daemon.sh
```

Then enter the dev shell:

```bash
nix develop
```

Common commands inside the shell:

```bash
go test ./...
go run ./cmd/estuary
golangci-lint run
```

You can also run commands without entering an interactive shell:

```bash
nix develop -c go test ./...
```

## Config

Estuary config lives at `~/.config/estuary/config.yaml`. It becomes the source of truth after initial import from existing provider configs.

Shared commands live at `~/.config/estuary/commands/` as individual Markdown files with frontmatter:

```markdown
---
name: plan-work
description: Turn a rough task into an implementation plan
providers: [claude, codex]
---
Analyze the current repository state and produce a concrete implementation plan.
```

These are synced into provider-native command folders on every Estuary startup.

Runtime data lives at `~/.estuary/data/estuary.db`.

## Scope

Check [FEATURES.md](./FEATURES.md) for the authoritative shipped feature list and current implementation status.
