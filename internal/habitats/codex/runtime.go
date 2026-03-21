package codex

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"strings"

	"github.com/brianmeier/estuary/internal/domain"
)

type Runtime struct{}

func New() *Runtime {
	return &Runtime{}
}

func (r *Runtime) ExecuteTurnStream(ctx context.Context, session domain.Session, prompt string, emit func(domain.TurnEvent) error) error {
	var args []string
	if session.NativeSessionID != "" {
		args = []string{"exec", "resume", session.NativeSessionID, prompt}
	} else {
		args = []string{"exec", prompt}
	}
	args = append(args,
		"--json",
		"--skip-git-repo-check",
		"-C", session.FolderPath,
	)
	if strings.TrimSpace(session.CurrentModel) != "" {
		args = append(args, "-m", session.CurrentModel)
	}
	if sandbox := nativeSetting(session.ResolvedBoundarySettings, "sandbox_mode", "workspace-write"); sandbox != "" {
		args = append(args, "-s", sandbox)
	}
	if approval := nativeSetting(session.ResolvedBoundarySettings, "approval_policy", "on-request"); approval != "" {
		args = append(args, "-a", approval)
	}

	cmd := exec.CommandContext(ctx, "codex", args...)
	cmd.Dir = session.FolderPath

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		return err
	}
	if emit != nil {
		if err := emit(domain.TurnEvent{Kind: domain.TurnEventStarted, SessionID: session.ID}); err != nil {
			return err
		}
	}

	parser := &parser{sessionID: session.ID}
	readErr := streamJSONLines(stdout, func(line string) error {
		return parser.handleLine(line, emit)
	})
	waitErr := cmd.Wait()
	if readErr != nil {
		return readErr
	}
	if waitErr != nil {
		return parser.commandError(waitErr, stderr.String(), emit)
	}
	if emit != nil {
		return emit(domain.TurnEvent{
			Kind:            domain.TurnEventCompleted,
			SessionID:       session.ID,
			NativeSessionID: parser.nativeSessionID,
			Text:            strings.TrimSpace(parser.assistantText),
		})
	}
	return nil
}

type parser struct {
	sessionID       string
	nativeSessionID string
	assistantText   string
}

func (p *parser) handleLine(line string, emit func(domain.TurnEvent) error) error {
	var event map[string]any
	if err := json.Unmarshal([]byte(line), &event); err != nil {
		return nil
	}
	switch event["type"] {
	case "thread.started":
		if id, _ := event["thread_id"].(string); id != "" {
			p.nativeSessionID = id
		}
	case "agent_message_delta", "response.output_text.delta":
		text := strings.TrimSpace(stringify(event["delta"]))
		if text != "" {
			p.assistantText += text
			if emit != nil {
				return emit(domain.TurnEvent{Kind: domain.TurnEventDelta, SessionID: p.sessionID, NativeSessionID: p.nativeSessionID, Text: text})
			}
		}
	case "item.started":
		if item, ok := event["item"].(map[string]any); ok && isToolItem(item) && emit != nil {
			return emit(domain.TurnEvent{Kind: domain.TurnEventToolStarted, SessionID: p.sessionID, ToolName: toolName(item), Text: stringify(item)})
		}
	case "item.updated":
		if item, ok := event["item"].(map[string]any); ok && isToolItem(item) && emit != nil {
			return emit(domain.TurnEvent{Kind: domain.TurnEventToolOutput, SessionID: p.sessionID, ToolName: toolName(item), Text: stringify(item)})
		}
	case "item.completed":
		if item, ok := event["item"].(map[string]any); ok {
			if item["type"] == "agent_message" {
				if text, _ := item["text"].(string); text != "" {
					p.assistantText = strings.TrimSpace(text)
				}
				return nil
			}
			if isToolItem(item) && emit != nil {
				return emit(domain.TurnEvent{Kind: domain.TurnEventToolFinished, SessionID: p.sessionID, ToolName: toolName(item), Text: stringify(item)})
			}
		}
	case "error":
		message, _ := event["message"].(string)
		return p.reportError(message, emit)
	case "turn.failed":
		if payload, ok := event["error"].(map[string]any); ok {
			return p.reportError(stringify(payload["message"]), emit)
		}
	}
	return nil
}

func (p *parser) reportError(message string, emit func(domain.TurnEvent) error) error {
	text := strings.TrimSpace(message)
	if text == "" {
		text = "codex execution failed"
	}
	metadata := map[string]string{}
	if resumeRejected(text) {
		metadata["resume_rejected"] = "true"
	}
	if emit != nil {
		_ = emit(domain.TurnEvent{Kind: domain.TurnEventHabitatError, SessionID: p.sessionID, NativeSessionID: p.nativeSessionID, Text: text, Metadata: metadata})
	}
	return fmt.Errorf("%s", text)
}

func (p *parser) commandError(waitErr error, stderr string, emit func(domain.TurnEvent) error) error {
	text := strings.TrimSpace(stderr)
	if text == "" {
		text = waitErr.Error()
	}
	return p.reportError(text, emit)
}

func streamJSONLines(reader io.Reader, handle func(string) error) error {
	scanner := bufio.NewScanner(reader)
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		if err := handle(line); err != nil {
			return err
		}
	}
	return scanner.Err()
}

func isToolItem(item map[string]any) bool {
	typ, _ := item["type"].(string)
	return strings.Contains(typ, "tool")
}

func toolName(item map[string]any) string {
	if name, _ := item["name"].(string); name != "" {
		return name
	}
	if typ, _ := item["type"].(string); typ != "" {
		return typ
	}
	return "tool"
}

func stringify(v any) string {
	switch value := v.(type) {
	case string:
		return value
	default:
		b, _ := json.Marshal(value)
		return string(b)
	}
}

func resumeRejected(message string) bool {
	message = strings.ToLower(message)
	return strings.Contains(message, "resume") && (strings.Contains(message, "not found") || strings.Contains(message, "invalid") || strings.Contains(message, "expired"))
}

func nativeSetting(settingsJSON, key, fallback string) string {
	if settingsJSON == "" {
		return fallback
	}
	var settings map[string]string
	if err := json.Unmarshal([]byte(settingsJSON), &settings); err != nil {
		return fallback
	}
	if value := strings.TrimSpace(settings[key]); value != "" {
		return value
	}
	return fallback
}
