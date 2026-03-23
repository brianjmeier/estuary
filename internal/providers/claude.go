package providers

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"

	"github.com/brianmeier/estuary/internal/domain"
)

type ClaudeAdapter struct {
	mu       sync.Mutex
	runtimes map[string]*claudeSDKRuntime
}

func NewClaudeAdapter() *ClaudeAdapter {
	return &ClaudeAdapter{runtimes: map[string]*claudeSDKRuntime{}}
}

func (a *ClaudeAdapter) StartProviderSession(ctx context.Context, session domain.Session) (domain.ProviderSessionRef, domain.ProviderProcessState, error) {
	rt, err := startClaudeSDKRuntime(ctx, session, nil)
	if err != nil {
		return domain.ProviderSessionRef{}, domain.ProviderProcessState{}, err
	}
	a.mu.Lock()
	a.runtimes[session.ID] = rt
	a.mu.Unlock()
	ref := domain.ProviderSessionRef{
		ID:                       uuid.NewString(),
		SessionID:                session.ID,
		Provider:                 domain.HabitatClaude,
		RuntimeKind:              domain.SessionRuntimeKindProviderSession,
		ProviderSessionID:        rt.sessionID(),
		ProviderThreadID:         rt.sessionID(),
		ProviderResumeCursorJSON: rt.resumeCursorJSON(),
		Status:                   domain.ProviderRuntimeStatusReady,
		StartedAt:                time.Now().UTC(),
		UpdatedAt:                time.Now().UTC(),
	}
	return ref, rt.processState(ref.ID), nil
}

func (a *ClaudeAdapter) ResumeProviderSession(ctx context.Context, session domain.Session, ref domain.ProviderSessionRef) (domain.ProviderSessionRef, domain.ProviderProcessState, error) {
	var resumeCursor map[string]any
	if ref.ProviderResumeCursorJSON != "" {
		_ = json.Unmarshal([]byte(ref.ProviderResumeCursorJSON), &resumeCursor)
		if nested, ok := resumeCursor["resume_cursor"].(map[string]any); ok && len(nested) > 0 {
			resumeCursor = nested
		}
	}
	rt, err := startClaudeSDKRuntime(ctx, session, resumeCursor)
	if err != nil {
		return ref, domain.ProviderProcessState{}, err
	}
	a.mu.Lock()
	a.runtimes[session.ID] = rt
	a.mu.Unlock()
	ref.ProviderSessionID = rt.sessionID()
	ref.ProviderThreadID = rt.sessionID()
	ref.ProviderResumeCursorJSON = rt.resumeCursorJSON()
	ref.Status = domain.ProviderRuntimeStatusReady
	ref.UpdatedAt = time.Now().UTC()
	return ref, rt.processState(ref.ID), nil
}

func (a *ClaudeAdapter) SendProviderTurn(ctx context.Context, session domain.Session, ref domain.ProviderSessionRef, prompt string, emit func(domain.TurnEvent) error) (domain.ProviderSessionRef, domain.ProviderProcessState, error) {
	rt, err := a.runtime(session.ID)
	if err != nil {
		return ref, domain.ProviderProcessState{}, err
	}
	turnID := uuid.NewString()
	if err := rt.sendCommand(map[string]any{
		"op":     "send",
		"turnId": turnID,
		"prompt": prompt,
	}); err != nil {
		return ref, rt.processState(ref.ID), err
	}
	var assistantText strings.Builder
	for {
		select {
		case <-ctx.Done():
			return ref, rt.processState(ref.ID), ctx.Err()
		case msg := <-rt.notifications:
			if tid := stringValue(msg["turnId"]); tid != "" && tid != turnID {
				continue
			}
			if sid := stringValue(msg["sessionId"]); sid != "" {
				rt.setSessionID(sid)
			}
			switch stringValue(msg["event"]) {
			case "turn.started":
				if emit != nil {
					if err := emit(domain.TurnEvent{Kind: domain.TurnEventStarted, SessionID: session.ID, TurnID: turnID, NativeSessionID: rt.sessionID()}); err != nil {
						return ref, rt.processState(ref.ID), err
					}
				}
			case "delta":
				text := stringValue(msg["text"])
				assistantText.WriteString(text)
				if emit != nil {
					if err := emit(domain.TurnEvent{Kind: domain.TurnEventDelta, SessionID: session.ID, TurnID: turnID, NativeSessionID: rt.sessionID(), Text: text}); err != nil {
						return ref, rt.processState(ref.ID), err
					}
				}
			case "tool.started":
				if emit != nil {
					if err := emit(domain.TurnEvent{Kind: domain.TurnEventToolStarted, SessionID: session.ID, TurnID: turnID, NativeSessionID: rt.sessionID(), ToolName: firstNonEmpty(stringValue(msg["toolName"]), "tool"), Text: mustJSON(msg["payload"])}); err != nil {
						return ref, rt.processState(ref.ID), err
					}
				}
			case "tool.output":
				if emit != nil {
					if err := emit(domain.TurnEvent{Kind: domain.TurnEventToolOutput, SessionID: session.ID, TurnID: turnID, NativeSessionID: rt.sessionID(), Text: stringValue(msg["text"])}); err != nil {
						return ref, rt.processState(ref.ID), err
					}
				}
			case "tool.finished":
				if strings.EqualFold(stringValue(msg["toolName"]), "TodoWrite") && emit != nil {
					for _, taskEvent := range claudeTodoEvents(session.ID, turnID, rt.sessionID(), msg["payload"]) {
						if err := emit(taskEvent); err != nil {
							return ref, rt.processState(ref.ID), err
						}
					}
				}
				if emit != nil {
					if err := emit(domain.TurnEvent{Kind: domain.TurnEventToolFinished, SessionID: session.ID, TurnID: turnID, NativeSessionID: rt.sessionID(), ToolName: firstNonEmpty(stringValue(msg["toolName"]), "tool"), Text: mustJSON(msg["payload"])}); err != nil {
						return ref, rt.processState(ref.ID), err
					}
				}
			case "task.progress":
				if emit != nil {
					if err := emit(domain.TurnEvent{
						Kind:            domain.TurnEventTaskProgress,
						SessionID:       session.ID,
						TurnID:          turnID,
						NativeSessionID: rt.sessionID(),
						TaskID:          stringValue(msg["taskId"]),
						TaskTitle:       stringValue(msg["title"]),
						TaskDetail:      stringValue(msg["detail"]),
						TaskStatus:      "running",
					}); err != nil {
						return ref, rt.processState(ref.ID), err
					}
				}
			case "turn.error", "runtime.error":
				text := firstNonEmpty(stringValue(msg["error"]), stringValue(msg["message"]), stringValue(msg["result"]), "claude sdk bridge failed")
				if emit != nil {
					_ = emit(domain.TurnEvent{Kind: domain.TurnEventHabitatError, SessionID: session.ID, TurnID: turnID, NativeSessionID: rt.sessionID(), Text: text})
				}
				return ref, rt.processState(ref.ID), fmt.Errorf("%s", text)
			case "turn.completed":
				rt.updateResumeCursor(msg["resumeCursor"])
				ref.ProviderSessionID = rt.sessionID()
				ref.ProviderThreadID = rt.sessionID()
				ref.ProviderResumeCursorJSON = rt.resumeCursorJSON()
				ref.UpdatedAt = time.Now().UTC()
				if emit != nil {
					if err := emit(domain.TurnEvent{Kind: domain.TurnEventCompleted, SessionID: session.ID, TurnID: turnID, NativeSessionID: rt.sessionID(), Text: strings.TrimSpace(assistantText.String())}); err != nil {
						return ref, rt.processState(ref.ID), err
					}
				}
				return ref, rt.processState(ref.ID), nil
			}
		}
	}
}

func (a *ClaudeAdapter) InterruptProviderTurn(_ context.Context, session domain.Session, ref domain.ProviderSessionRef, turnID string) error {
	rt, err := a.runtime(session.ID)
	if err != nil {
		return err
	}
	return rt.sendCommand(map[string]any{"op": "interrupt", "turnId": turnID})
}

func (a *ClaudeAdapter) CloseProviderSession(_ context.Context, ref domain.ProviderSessionRef) error {
	a.mu.Lock()
	rt := a.runtimes[ref.SessionID]
	delete(a.runtimes, ref.SessionID)
	a.mu.Unlock()
	if rt == nil {
		return nil
	}
	_ = rt.sendCommand(map[string]any{"op": "close"})
	return rt.close()
}

func (a *ClaudeAdapter) runtime(sessionID string) (*claudeSDKRuntime, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	rt := a.runtimes[sessionID]
	if rt == nil {
		return nil, fmt.Errorf("claude runtime is not connected")
	}
	return rt, nil
}

type claudeSDKRuntime struct {
	cmd           *exec.Cmd
	stdin         io.WriteCloser
	notifications chan map[string]any
	resumeCursor  atomic.Value
	currentID     atomic.Value
	closed        atomic.Bool
}

func startClaudeSDKRuntime(ctx context.Context, session domain.Session, resumeCursor map[string]any) (*claudeSDKRuntime, error) {
	repoRoot, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	scriptPath := filepath.Join(repoRoot, "internal", "providers", "claude_sdk_bridge.mjs")
	cmd := exec.CommandContext(ctx, "node", scriptPath)
	cmd.Dir = repoRoot
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, err
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	rt := &claudeSDKRuntime{
		cmd:           cmd,
		stdin:         stdin,
		notifications: make(chan map[string]any, 512),
	}
	go rt.readLoop(stdout)
	go rt.readLoop(stderr)
	sessionID := session.ID
	if resumeCursor != nil {
		if value := firstNonEmpty(
			stringValue(resumeCursor["resume"]),
			stringValue(resumeCursor["sessionId"]),
			stringValue(resumeCursor["provider_session_id"]),
		); value != "" {
			sessionID = value
		}
	}
	rt.currentID.Store(sessionID)
	if resumeCursor != nil {
		rt.resumeCursor.Store(resumeCursor)
	}
	command := map[string]any{
		"op":             "start",
		"sessionId":      session.ID,
		"cwd":            session.FolderPath,
		"model":          session.CurrentModel,
		"permissionMode": claudePermissionMode(session.ResolvedBoundarySettings),
	}
	if resumeCursor != nil {
		if value := firstNonEmpty(
			stringValue(resumeCursor["resume"]),
			stringValue(resumeCursor["sessionId"]),
			stringValue(resumeCursor["provider_session_id"]),
		); value != "" {
			command["resume"] = value
		}
		if value := stringValue(resumeCursor["resumeSessionAt"]); value != "" {
			command["resumeSessionAt"] = value
		}
	}
	if err := rt.sendCommand(command); err != nil {
		return nil, err
	}
	return rt, nil
}

func (r *claudeSDKRuntime) sendCommand(command map[string]any) error {
	raw, _ := json.Marshal(command)
	_, err := io.WriteString(r.stdin, string(raw)+"\n")
	return err
}

func (r *claudeSDKRuntime) readLoop(reader io.Reader) {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var msg map[string]any
		if err := json.Unmarshal([]byte(line), &msg); err != nil {
			continue
		}
		r.notifications <- msg
	}
}

func (r *claudeSDKRuntime) processState(providerSessionRefID string) domain.ProviderProcessState {
	pid := 0
	if r.cmd != nil && r.cmd.Process != nil {
		pid = r.cmd.Process.Pid
	}
	return domain.ProviderProcessState{
		ID:                uuid.NewString(),
		ProviderSessionID: providerSessionRefID,
		Transport:         "claude_agent_sdk_bridge",
		Warm:              !r.closed.Load(),
		PID:               pid,
		Connected:         !r.closed.Load(),
		MetadataJSON:      "{}",
		CreatedAt:         time.Now().UTC(),
		UpdatedAt:         time.Now().UTC(),
	}
}

func (r *claudeSDKRuntime) close() error {
	if r.closed.Swap(true) {
		return nil
	}
	if r.stdin != nil {
		_ = r.stdin.Close()
	}
	if r.cmd != nil && r.cmd.Process != nil {
		_ = r.cmd.Process.Kill()
		_, _ = r.cmd.Process.Wait()
	}
	return nil
}

func (r *claudeSDKRuntime) updateResumeCursor(value any) {
	if payload, ok := value.(map[string]any); ok {
		r.resumeCursor.Store(payload)
		if sessionID, ok := payload["resume"].(string); ok && sessionID != "" {
			r.currentID.Store(sessionID)
		}
	}
}

func (r *claudeSDKRuntime) resumeCursorJSON() string {
	value, _ := r.resumeCursor.Load().(map[string]any)
	if value == nil {
		return ""
	}
	b, _ := json.Marshal(value)
	return string(b)
}

func (r *claudeSDKRuntime) sessionID() string {
	value, _ := r.currentID.Load().(string)
	return value
}

func (r *claudeSDKRuntime) setSessionID(value string) {
	if strings.TrimSpace(value) != "" {
		r.currentID.Store(value)
	}
}

func claudePermissionMode(settingsJSON string) string {
	if settingsJSON == "" {
		return "default"
	}
	var settings map[string]string
	if err := json.Unmarshal([]byte(settingsJSON), &settings); err != nil {
		return "default"
	}
	return firstNonEmpty(settings["permission_mode"], "default")
}

func claudeTodoEvents(sessionID, turnID, nativeSessionID string, payload any) []domain.TurnEvent {
	root, _ := payload.(map[string]any)
	if root == nil {
		return nil
	}
	input := root
	if nested, ok := root["input"].(map[string]any); ok {
		input = nested
	}
	items, ok := input["todos"].([]any)
	if !ok {
		return nil
	}
	var out []domain.TurnEvent
	for _, item := range items {
		todo, ok := item.(map[string]any)
		if !ok {
			continue
		}
		taskID := firstNonEmpty(stringValue(todo["id"]), stringValue(todo["content"]), stringValue(todo["activeForm"]))
		title := firstNonEmpty(stringValue(todo["content"]), stringValue(todo["activeForm"]), taskID)
		status := firstNonEmpty(stringValue(todo["status"]), "running")
		kind := domain.TurnEventTaskProgress
		switch strings.ToLower(status) {
		case "completed", "done":
			kind = domain.TurnEventTaskComplete
		case "pending", "in_progress", "running":
			kind = domain.TurnEventTaskStarted
		}
		out = append(out, domain.TurnEvent{
			Kind:            kind,
			SessionID:       sessionID,
			TurnID:          turnID,
			NativeSessionID: nativeSessionID,
			TaskID:          taskID,
			TaskTitle:       title,
			TaskDetail:      firstNonEmpty(stringValue(todo["activeForm"]), stringValue(todo["status"])),
			TaskStatus:      status,
		})
	}
	return out
}
