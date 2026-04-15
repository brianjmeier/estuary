package app

import (
	"fmt"
	"io"

	"github.com/brianmeier/estuary/internal/domain"
)

type chromeState struct {
	Model   string
	Habitat domain.Habitat
	Folder  string
	Notice  string
}

type hostChrome interface {
	Apply(state chromeState)
	Clear()
}

func newHostChrome(out io.Writer) hostChrome {
	if tmux := newTmuxChromeFromEnv(); tmux != nil {
		return tmux
	}
	return &oscTitleChrome{out: out}
}

type oscTitleChrome struct {
	out       io.Writer
	lastTitle string
}

func (o *oscTitleChrome) Apply(state chromeState) {
	if o == nil || o.out == nil {
		return
	}
	title := formatChromeTitle(state)
	if title == o.lastTitle {
		return
	}
	o.lastTitle = title
	fmt.Fprintf(o.out, "\033]2;%s\a", title)
}

func (o *oscTitleChrome) Clear() {
	if o == nil || o.out == nil {
		return
	}
	o.lastTitle = ""
	fmt.Fprint(o.out, "\033]2;estuary\a")
}

func formatChromeTitle(state chromeState) string {
	if state.Notice != "" {
		return sanitizeTmuxTitle("◆ estuary | " + state.Notice)
	}
	return sanitizeTmuxTitle(fmt.Sprintf("◆ estuary | %s | %s | %s", state.Model, state.Habitat, state.Folder))
}
