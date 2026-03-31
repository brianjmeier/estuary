package main

import (
	"context"
	"log"
	"os"

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

	ts, err := app.NewTerminalSession(ctx, cwd, st, prober)
	if err != nil {
		log.Fatalf("create terminal session: %v", err)
	}

	if err := ts.Run(); err != nil && err.Error() != "quit" {
		log.Printf("estuary: %v", err)
	}
}
