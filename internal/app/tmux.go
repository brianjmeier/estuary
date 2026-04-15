package app

import (
	"os"
	"os/exec"
	"strings"
	"sync"
)

type tmuxChrome struct {
	paneID string

	mu        sync.Mutex
	lastTitle string
}

func newTmuxChromeFromEnv() *tmuxChrome {
	paneID := strings.TrimSpace(os.Getenv("TMUX_PANE"))
	if paneID == "" {
		return nil
	}
	return &tmuxChrome{paneID: paneID}
}

func (t *tmuxChrome) SetTitle(title string) {
	if t == nil || t.paneID == "" {
		return
	}

	title = sanitizeTmuxTitle(title)

	t.mu.Lock()
	if title == t.lastTitle {
		t.mu.Unlock()
		return
	}
	t.lastTitle = title
	t.mu.Unlock()

	cmd := exec.Command("tmux", "select-pane", "-t", t.paneID, "-T", title)
	_ = cmd.Run()
}

func (t *tmuxChrome) Apply(state chromeState) {
	if t == nil {
		return
	}
	t.SetTitle(formatChromeTitle(state))
}

func (t *tmuxChrome) Clear() {
	if t == nil {
		return
	}
	t.SetTitle("estuary")
}

func sanitizeTmuxTitle(title string) string {
	title = strings.ReplaceAll(title, "\n", " ")
	title = strings.ReplaceAll(title, "\r", " ")
	title = strings.TrimSpace(title)
	if title == "" {
		return "estuary"
	}
	return title
}
