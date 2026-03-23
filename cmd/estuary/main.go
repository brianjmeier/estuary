package main

import (
	"context"
	"log"
	"os"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/brianmeier/estuary/internal/app"
	"github.com/brianmeier/estuary/internal/prereq"
	"github.com/brianmeier/estuary/internal/store"
)

func main() {
	ctx := context.Background()
	cwd, err := os.Getwd()
	if err != nil {
		log.Fatalf("resolve cwd: %v", err)
	}

	dataDir, err := store.DefaultDataDir()
	if err != nil {
		log.Fatalf("resolve data dir: %v", err)
	}

	st, err := store.Open(ctx, dataDir)
	if err != nil {
		log.Fatalf("open store: %v", err)
	}
	defer func() {
		if err := st.Close(); err != nil {
			log.Printf("close store: %v", err)
		}
	}()

	prober := prereq.NewProber()

	m, err := app.NewModel(ctx, cwd, st, prober)
	if err != nil {
		log.Fatalf("create app model: %v", err)
	}

	p := tea.NewProgram(
		m,
		tea.WithAltScreen(),
		tea.WithMouseCellMotion(),
	)
	if _, err := p.Run(); err != nil {
		log.Printf("estuary exited with error: %v", err)
		os.Exit(1)
	}
}
