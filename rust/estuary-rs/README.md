# estuary-rs

This is the Rust rewrite foundation for Estuary.

It deliberately starts with the terminal/runtime slice:

- full PTY ownership for the child process
- host-managed chrome via tmux pane title or OSC window title
- minimal `Ctrl+K` leader controls
- explicit handoff packets for model switching

It does **not** yet replace the full Go application surface. The goal of this first cut is to prove the simpler terminal architecture and make future porting incremental.

## Run

```bash
cargo run -p estuary-rs
```

The current directory becomes the session folder. If an Estuary session already exists for that folder in `~/.estuary/data/estuary.db`, the runtime reuses it.

## Leader Keys

- `Ctrl+K ?` help
- `Ctrl+K s` switch session
- `Ctrl+K m` switch model
- `Ctrl+K r` reconnect
- `Ctrl+K q` quit

## Notes

- `Ctrl+C` is passed through to the child provider.
- Model switching prefers in-place switching when the provider supports it.
- Otherwise Estuary restarts the PTY with a saved handoff packet and injects structured continuation context.
