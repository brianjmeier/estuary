package providers

import "github.com/brianmeier/estuary/internal/domain"

// TerminalAdapter describes how to spawn and configure a native provider
// interactive terminal session (as opposed to the headless turn-stream adapters).
type TerminalAdapter interface {
	// Provider returns the habitat this adapter handles.
	Provider() domain.Habitat

	// StartArgs returns the command, extra args, and additional env vars for a
	// fresh interactive session.
	StartArgs(session domain.Session) (cmd string, args []string, env []string)

	// ResumeArgs returns args for resuming a prior native session by its ID.
	// Falls back to StartArgs behaviour when nativeID is empty.
	ResumeArgs(session domain.Session, nativeID string) (cmd string, args []string, env []string)

	// HandoffArgs returns args for starting a session that injects a handoff
	// packet as initial context. The packet text is already serialised.
	HandoffArgs(session domain.Session, packetText string) (cmd string, args []string, env []string)

	// ModelSwitchInput returns text to inject into PTY stdin to switch to the
	// given model within an active session. Returns "" when the provider does
	// not support runtime model switching (requires a PTY restart instead).
	ModelSwitchInput(modelID string) string
}
