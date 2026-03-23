package providers

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"

	"github.com/brianmeier/estuary/internal/domain"
)

type CodexAdapter struct {
	mu       sync.Mutex
	runtimes map[string]*codexRuntime
}

func NewCodexAdapter() *CodexAdapter {
	return &CodexAdapter{runtimes: map[string]*codexRuntime{}}
}

func (a *CodexAdapter) StartProviderSession(ctx context.Context, session domain.Session) (domain.ProviderSessionRef, domain.ProviderProcessState, error) {
	rt, err := startCodexRuntime(ctx)
	if err != nil {
		return domain.ProviderSessionRef{}, domain.ProviderProcessState{}, err
	}
	ref, err := rt.startThread(ctx, session)
	if err != nil {
		_ = rt.close()
		return domain.ProviderSessionRef{}, domain.ProviderProcessState{}, err
	}
	a.mu.Lock()
	a.runtimes[session.ID] = rt
	a.mu.Unlock()
	return ref, rt.processState(ref.ID), nil
}

func (a *CodexAdapter) ResumeProviderSession(ctx context.Context, session domain.Session, ref domain.ProviderSessionRef) (domain.ProviderSessionRef, domain.ProviderProcessState, error) {
	rt, err := startCodexRuntime(ctx)
	if err != nil {
		return ref, domain.ProviderProcessState{}, err
	}
	ref, err = rt.resumeThread(ctx, session, ref)
	if err != nil {
		_ = rt.close()
		return ref, domain.ProviderProcessState{}, err
	}
	a.mu.Lock()
	a.runtimes[session.ID] = rt
	a.mu.Unlock()
	return ref, rt.processState(ref.ID), nil
}

func (a *CodexAdapter) SendProviderTurn(ctx context.Context, session domain.Session, ref domain.ProviderSessionRef, prompt string, emit func(domain.TurnEvent) error) (domain.ProviderSessionRef, domain.ProviderProcessState, error) {
	rt, err := a.runtime(session.ID)
	if err != nil {
		return ref, domain.ProviderProcessState{}, err
	}
	ref, err = rt.sendTurn(ctx, session, ref, prompt, emit)
	return ref, rt.processState(ref.ID), err
}

func (a *CodexAdapter) InterruptProviderTurn(ctx context.Context, session domain.Session, ref domain.ProviderSessionRef, turnID string) error {
	rt, err := a.runtime(session.ID)
	if err != nil {
		return err
	}
	return rt.interruptTurn(ctx, ref, turnID)
}

func (a *CodexAdapter) CloseProviderSession(_ context.Context, ref domain.ProviderSessionRef) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	rt := a.runtimes[ref.SessionID]
	if rt == nil {
		return nil
	}
	delete(a.runtimes, ref.SessionID)
	return rt.close()
}

func (a *CodexAdapter) runtime(sessionID string) (*codexRuntime, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	rt := a.runtimes[sessionID]
	if rt == nil {
		return nil, fmt.Errorf("codex runtime is not connected")
	}
	return rt, nil
}

type codexRuntime struct {
	cmd           *exec.Cmd
	stdin         io.WriteCloser
	responses     map[string]chan map[string]any
	notifications chan map[string]any
	mu            sync.Mutex
	requestID     atomic.Int64
	closed        atomic.Bool
}

func startCodexRuntime(ctx context.Context) (*codexRuntime, error) {
	cmd := exec.CommandContext(ctx, "codex", "app-server", "--listen", "stdio://")
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
	rt := &codexRuntime{
		cmd:           cmd,
		stdin:         stdin,
		responses:     map[string]chan map[string]any{},
		notifications: make(chan map[string]any, 256),
	}
	go rt.readLoop(stdout)
	go rt.readLoop(stderr)
	if _, err := rt.request(ctx, "initialize", map[string]any{
		"clientInfo": map[string]any{
			"name":    "estuary",
			"version": "dev",
		},
	}); err != nil {
		_ = rt.close()
		return nil, err
	}
	return rt, nil
}

func (r *codexRuntime) startThread(ctx context.Context, session domain.Session) (domain.ProviderSessionRef, error) {
	result, err := r.request(ctx, "thread/start", map[string]any{
		"cwd":            session.FolderPath,
		"model":          session.CurrentModel,
		"sandbox":        codexNativeSetting(session.ResolvedBoundarySettings, "sandbox_mode", "workspace-write"),
		"approvalPolicy": codexNativeSetting(session.ResolvedBoundarySettings, "approval_policy", "on-request"),
	})
	if err != nil {
		return domain.ProviderSessionRef{}, err
	}
	thread := nestedMap(result, "thread")
	threadID := stringValue(thread["id"])
	ref := domain.ProviderSessionRef{
		ID:                       uuid.NewString(),
		SessionID:                session.ID,
		Provider:                 domain.HabitatCodex,
		RuntimeKind:              domain.SessionRuntimeKindProviderSession,
		ProviderSessionID:        threadID,
		ProviderThreadID:         threadID,
		ProviderResumeCursorJSON: mustJSON(map[string]any{"thread_path": stringValue(thread["path"])}),
		Status:                   domain.ProviderRuntimeStatusReady,
		StartedAt:                time.Now().UTC(),
		UpdatedAt:                time.Now().UTC(),
	}
	return ref, nil
}

func (r *codexRuntime) resumeThread(ctx context.Context, session domain.Session, ref domain.ProviderSessionRef) (domain.ProviderSessionRef, error) {
	result, err := r.request(ctx, "thread/resume", map[string]any{
		"threadId":       ref.ProviderThreadID,
		"cwd":            session.FolderPath,
		"model":          session.CurrentModel,
		"sandbox":        codexNativeSetting(session.ResolvedBoundarySettings, "sandbox_mode", "workspace-write"),
		"approvalPolicy": codexNativeSetting(session.ResolvedBoundarySettings, "approval_policy", "on-request"),
	})
	if err != nil {
		return ref, err
	}
	thread := nestedMap(result, "thread")
	if id := stringValue(thread["id"]); id != "" {
		ref.ProviderSessionID = id
		ref.ProviderThreadID = id
	}
	ref.Status = domain.ProviderRuntimeStatusReady
	ref.UpdatedAt = time.Now().UTC()
	return ref, nil
}

func (r *codexRuntime) sendTurn(ctx context.Context, session domain.Session, ref domain.ProviderSessionRef, prompt string, emit func(domain.TurnEvent) error) (domain.ProviderSessionRef, error) {
	result, err := r.request(ctx, "turn/start", map[string]any{
		"threadId": ref.ProviderThreadID,
		"input": []map[string]any{{
			"type": "text",
			"text": prompt,
		}},
	})
	if err != nil {
		return ref, err
	}
	turnID := stringValue(nestedMap(result, "turn")["id"])
	for {
		select {
		case <-ctx.Done():
			return ref, ctx.Err()
		case msg := <-r.notifications:
			if stringValue(msg["method"]) == "" {
				continue
			}
			method := stringValue(msg["method"])
			params := nestedMap(msg, "params")
			if tid := firstNonEmpty(stringValue(params["turnId"]), stringValue(nestedMap(params, "turn")["id"])); tid != "" && tid != turnID {
				continue
			}
			switch method {
			case "turn/started":
				if emit != nil {
					if err := emit(domain.TurnEvent{Kind: domain.TurnEventStarted, SessionID: session.ID, TurnID: turnID, NativeSessionID: ref.ProviderThreadID}); err != nil {
						return ref, err
					}
				}
			case "item/agentMessage/delta":
				if emit != nil {
					if err := emit(domain.TurnEvent{Kind: domain.TurnEventDelta, SessionID: session.ID, TurnID: turnID, NativeSessionID: ref.ProviderThreadID, Text: stringValue(params["delta"])}); err != nil {
						return ref, err
					}
				}
			case "item/started":
				item := nestedMap(params, "item")
				if toolName := codexToolName(item); toolName != "" && emit != nil {
					if err := emit(domain.TurnEvent{Kind: domain.TurnEventToolStarted, SessionID: session.ID, TurnID: turnID, NativeSessionID: ref.ProviderThreadID, ToolName: toolName, Text: mustJSON(item)}); err != nil {
						return ref, err
					}
				}
			case "item/completed":
				item := nestedMap(params, "item")
				if toolName := codexToolName(item); toolName != "" && emit != nil {
					if err := emit(domain.TurnEvent{Kind: domain.TurnEventToolFinished, SessionID: session.ID, TurnID: turnID, NativeSessionID: ref.ProviderThreadID, ToolName: toolName, Text: mustJSON(item)}); err != nil {
						return ref, err
					}
				}
			case "codex/event/task_started":
				payload := nestedMap(params, "payload")
				if emit != nil {
					if err := emit(domain.TurnEvent{
						Kind:            domain.TurnEventTaskStarted,
						SessionID:       session.ID,
						TurnID:          turnID,
						NativeSessionID: ref.ProviderThreadID,
						TaskID:          firstNonEmpty(stringValue(payload["id"]), stringValue(params["taskId"])),
						TaskTitle:       firstNonEmpty(stringValue(payload["description"]), stringValue(payload["title"]), "Task"),
						TaskStatus:      "running",
					}); err != nil {
						return ref, err
					}
				}
			case "codex/event/task_progress":
				payload := nestedMap(params, "payload")
				if emit != nil {
					if err := emit(domain.TurnEvent{
						Kind:            domain.TurnEventTaskProgress,
						SessionID:       session.ID,
						TurnID:          turnID,
						NativeSessionID: ref.ProviderThreadID,
						TaskID:          firstNonEmpty(stringValue(payload["id"]), stringValue(params["taskId"])),
						TaskTitle:       firstNonEmpty(stringValue(payload["description"]), stringValue(payload["title"]), "Task"),
						TaskDetail:      stringValue(payload["description"]),
						TaskStatus:      "running",
					}); err != nil {
						return ref, err
					}
				}
			case "codex/event/task_complete":
				payload := nestedMap(params, "payload")
				if emit != nil {
					if err := emit(domain.TurnEvent{
						Kind:            domain.TurnEventTaskComplete,
						SessionID:       session.ID,
						TurnID:          turnID,
						NativeSessionID: ref.ProviderThreadID,
						TaskID:          firstNonEmpty(stringValue(payload["id"]), stringValue(params["taskId"])),
						TaskTitle:       firstNonEmpty(stringValue(payload["description"]), stringValue(payload["title"]), "Task"),
						TaskDetail:      firstNonEmpty(stringValue(payload["last_agent_message"]), stringValue(payload["summary"])),
						TaskStatus:      "completed",
					}); err != nil {
						return ref, err
					}
				}
			case "error":
				text := stringValue(nestedMap(params, "error")["message"])
				if text == "" {
					text = mustJSON(params)
				}
				if emit != nil {
					_ = emit(domain.TurnEvent{Kind: domain.TurnEventHabitatError, SessionID: session.ID, TurnID: turnID, NativeSessionID: ref.ProviderThreadID, Text: text})
				}
				return ref, fmt.Errorf("%s", text)
			case "turn/completed":
				ref.UpdatedAt = time.Now().UTC()
				if emit != nil {
					if err := emit(domain.TurnEvent{Kind: domain.TurnEventCompleted, SessionID: session.ID, TurnID: turnID, NativeSessionID: ref.ProviderThreadID}); err != nil {
						return ref, err
					}
				}
				return ref, nil
			}
		}
	}
}

func (r *codexRuntime) interruptTurn(ctx context.Context, ref domain.ProviderSessionRef, turnID string) error {
	_, err := r.request(ctx, "turn/interrupt", map[string]any{
		"threadId": ref.ProviderThreadID,
		"turnId":   turnID,
	})
	return err
}

func (r *codexRuntime) request(ctx context.Context, method string, params map[string]any) (map[string]any, error) {
	id := strconv.FormatInt(r.requestID.Add(1), 10)
	responseCh := make(chan map[string]any, 1)
	r.mu.Lock()
	r.responses[id] = responseCh
	r.mu.Unlock()

	payload := map[string]any{"jsonrpc": "2.0", "id": id, "method": method, "params": params}
	raw, _ := json.Marshal(payload)
	if _, err := io.WriteString(r.stdin, string(raw)+"\n"); err != nil {
		return nil, err
	}

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case msg := <-responseCh:
		if errPayload, ok := msg["error"].(map[string]any); ok {
			return nil, fmt.Errorf("%s", stringValue(errPayload["message"]))
		}
		return nestedMap(msg, "result"), nil
	}
}

func (r *codexRuntime) readLoop(reader io.Reader) {
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
		if id := stringValue(msg["id"]); id != "" {
			r.mu.Lock()
			ch := r.responses[id]
			delete(r.responses, id)
			r.mu.Unlock()
			if ch != nil {
				ch <- msg
			}
			continue
		}
		r.notifications <- msg
	}
}

func (r *codexRuntime) processState(providerSessionRefID string) domain.ProviderProcessState {
	pid := 0
	if r.cmd != nil && r.cmd.Process != nil {
		pid = r.cmd.Process.Pid
	}
	return domain.ProviderProcessState{
		ID:                uuid.NewString(),
		ProviderSessionID: providerSessionRefID,
		Transport:         "codex_app_server_stdio",
		Warm:              !r.closed.Load(),
		PID:               pid,
		Connected:         !r.closed.Load(),
		MetadataJSON:      "{}",
		CreatedAt:         time.Now().UTC(),
		UpdatedAt:         time.Now().UTC(),
	}
}

func (r *codexRuntime) close() error {
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

func codexToolName(item map[string]any) string {
	typ := strings.ToLower(stringValue(item["type"]))
	if typ == "" || typ == "usermessage" || typ == "agentmessage" || typ == "reasoning" {
		return ""
	}
	return firstNonEmpty(stringValue(item["title"]), stringValue(item["name"]), typ)
}

func codexNativeSetting(settingsJSON, key, fallback string) string {
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

func nestedMap(parent map[string]any, key string) map[string]any {
	if parent == nil {
		return nil
	}
	value, _ := parent[key].(map[string]any)
	return value
}

func stringValue(v any) string {
	switch value := v.(type) {
	case string:
		return value
	default:
		return ""
	}
}

func mustJSON(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}
