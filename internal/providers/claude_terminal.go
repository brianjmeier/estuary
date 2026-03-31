package providers

import "github.com/brianmeier/estuary/internal/domain"

// ClaudeTerminalAdapter spawns and configures interactive Claude Code sessions.
type ClaudeTerminalAdapter struct{}

func (a *ClaudeTerminalAdapter) Provider() domain.Habitat {
	return domain.HabitatClaude
}

// StartArgs returns the command and args for a fresh Claude Code interactive session.
func (a *ClaudeTerminalAdapter) StartArgs(_ domain.Session) (string, []string, []string) {
	return "claude", nil, nil
}

// ResumeArgs returns args for resuming a prior Claude Code session by its native session ID.
func (a *ClaudeTerminalAdapter) ResumeArgs(session domain.Session, nativeID string) (string, []string, []string) {
	if nativeID == "" {
		return a.StartArgs(session)
	}
	return "claude", []string{"--resume", nativeID}, nil
}

// HandoffArgs returns args for a fresh session. Handoff injection is handled
// at the application layer by writing initial input to the PTY after the process starts.
func (a *ClaudeTerminalAdapter) HandoffArgs(session domain.Session, _ string) (string, []string, []string) {
	return a.StartArgs(session)
}

// ModelSwitchInput returns the /model slash command that switches Claude Code's
// active model within a running session.
func (a *ClaudeTerminalAdapter) ModelSwitchInput(modelID string) string {
	return "/model " + modelID + "\n"
}
