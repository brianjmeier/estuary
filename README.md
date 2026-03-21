# Estuary

Estuary is a local Go TUI for running Claude Code and Codex sessions behind one current-directory-first chat interface without replacing either habitat's native behavior.

The current implemented feature inventory lives in [FEATURES.md](/Users/brianmeier/dev/agenator/FEATURES.md). Treat that file as the authoritative shipped-feature index.

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
source /nix/var/nix/profiles/default/etc/profile.d/nix-daemon.sh
nix develop -c go test ./...
```

## Scope

The current shell now opens directly into a fresh session for the current working directory, defaults that session to `claude-sonnet-4-6`, and keeps the main screen focused on transcript plus composer. Sessions, settings, model changes, boundary controls, and traits live behind `Ctrl+K` instead of permanent sidebars.

Check [FEATURES.md](/Users/brianmeier/dev/agenator/FEATURES.md) for the authoritative shipped feature list and current implementation status.
