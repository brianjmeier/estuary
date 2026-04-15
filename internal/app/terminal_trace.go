package app

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

const terminalTraceEnvVar = "ESTUARY_TERMINAL_TRACE_FILE"

type terminalTrace struct {
	mu   sync.Mutex
	file *os.File
}

func newTerminalTraceFromEnv() (*terminalTrace, error) {
	path := strings.TrimSpace(os.Getenv(terminalTraceEnvVar))
	if path == "" {
		return nil, nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("create trace dir: %w", err)
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, fmt.Errorf("open trace file: %w", err)
	}
	t := &terminalTrace{file: f}
	t.Logf("trace started")
	return t, nil
}

func (t *terminalTrace) Close() error {
	if t == nil || t.file == nil {
		return nil
	}
	t.Logf("trace closed")
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.file.Close()
}

func (t *terminalTrace) Logf(format string, args ...any) {
	if t == nil || t.file == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	line := fmt.Sprintf(format, args...)
	fmt.Fprintf(t.file, "%s %s\n", time.Now().Format(time.RFC3339Nano), line)
}

func traceBytes(data []byte) string {
	if len(data) == 0 {
		return `""`
	}
	return strconv.QuoteToASCII(string(data))
}
