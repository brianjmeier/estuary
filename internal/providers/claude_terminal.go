package providers

import "github.com/brianmeier/estuary/internal/domain"

// ClaudeTerminalAdapter spawns and configures interactive Claude Code sessions.
type ClaudeTerminalAdapter struct{}

func (a *ClaudeTerminalAdapter) Provider() domain.Habitat {
	return domain.HabitatClaude
}

// StartArgs returns the command and args for a fresh Claude Code interactive session.
func (a *ClaudeTerminalAdapter) StartArgs(session domain.Session) (string, []string, []string) {
	args := []string{}
	if session.CurrentModel != "" {
		args = append(args, "--model", session.CurrentModel)
	}
	return "claude", args, nil
}

// ResumeArgs returns args for resuming a prior Claude Code session by its native session ID.
func (a *ClaudeTerminalAdapter) ResumeArgs(session domain.Session, nativeID string) (string, []string, []string) {
	if nativeID == "" {
		return a.StartArgs(session)
	}
	return "claude", []string{"--resume", nativeID}, nil
}

// HandoffArgs starts Claude with handoff context in the startup system prompt,
// leaving the interactive input available immediately.
func (a *ClaudeTerminalAdapter) HandoffArgs(session domain.Session, packetText string) (string, []string, []string) {
	cmd, args, env := a.StartArgs(session)
	if packetText != "" {
		args = append(args, "--append-system-prompt", packetText)
	}
	return cmd, args, env
}

// ModelSwitchInput is intentionally disabled. Same-provider model switches stay
// in the provider-native UI instead of Estuary typing slash commands into it.
func (a *ClaudeTerminalAdapter) ModelSwitchInput(_ string) string { return "" }
