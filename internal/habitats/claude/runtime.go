package claude

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
	args := []string{
		"-p",
		"--verbose",
		"--include-partial-messages",
		"--output-format", "stream-json",
	}
	if strings.TrimSpace(session.CurrentModel) != "" {
		args = append(args, "--model", session.CurrentModel)
	}
	if session.NativeSessionID != "" {
		args = append(args, "--resume", session.NativeSessionID)
	} else {
		args = append(args, "--session-id", session.ID)
	}
	args = append(args, prompt)

	cmd := exec.CommandContext(ctx, "claude", args...)
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
	if id, _ := event["session_id"].(string); id != "" {
		p.nativeSessionID = id
	}

	switch event["type"] {
	case "assistant":
		message, _ := event["message"].(map[string]any)
		text := extractAssistantText(message)
		if delta := strings.TrimPrefix(text, p.assistantText); strings.TrimSpace(delta) != "" {
			p.assistantText = text
			if emit != nil {
				return emit(domain.TurnEvent{
					Kind:            domain.TurnEventDelta,
					SessionID:       p.sessionID,
					NativeSessionID: p.nativeSessionID,
					Text:            delta,
				})
			}
		} else if text != "" {
			p.assistantText = text
		}
	case "system":
		if subtype, _ := event["subtype"].(string); strings.Contains(subtype, "tool") {
			text := strings.TrimSpace(stringify(event["message"]))
			if text != "" && emit != nil {
				return emit(domain.TurnEvent{Kind: domain.TurnEventToolOutput, SessionID: p.sessionID, Text: text})
			}
		}
	case "result":
		if isErr, _ := event["is_error"].(bool); isErr {
			message, _ := event["result"].(string)
			metadata := map[string]string{}
			if resumeRejected(message) {
				metadata["resume_rejected"] = "true"
			}
			if emit != nil {
				_ = emit(domain.TurnEvent{Kind: domain.TurnEventHabitatError, SessionID: p.sessionID, NativeSessionID: p.nativeSessionID, Text: message, Metadata: metadata})
			}
			return fmt.Errorf("%s", message)
		}
	}
	return nil
}

func (p *parser) commandError(waitErr error, stderr string, emit func(domain.TurnEvent) error) error {
	text := strings.TrimSpace(stderr)
	if text == "" {
		text = waitErr.Error()
	}
	metadata := map[string]string{}
	if resumeRejected(text) {
		metadata["resume_rejected"] = "true"
	}
	if emit != nil {
		_ = emit(domain.TurnEvent{
			Kind:            domain.TurnEventHabitatError,
			SessionID:       p.sessionID,
			NativeSessionID: p.nativeSessionID,
			Text:            text,
			Metadata:        metadata,
			Err:             waitErr,
		})
	}
	return fmt.Errorf("%s", text)
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

func extractAssistantText(message map[string]any) string {
	content, ok := message["content"].([]any)
	if !ok {
		return ""
	}
	var parts []string
	for _, item := range content {
		block, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if block["type"] == "text" {
			if text, _ := block["text"].(string); text != "" {
				parts = append(parts, text)
			}
		}
	}
	return strings.TrimSpace(strings.Join(parts, "\n"))
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

