package prereq

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/brianmeier/estuary/internal/domain"
	"github.com/brianmeier/estuary/internal/habitats"
)

type Prober struct{}

func NewProber() *Prober {
	return &Prober{}
}

func (p *Prober) ProbeAll(ctx context.Context) []domain.HabitatHealth {
	reg := habitats.Registry()
	out := make([]domain.HabitatHealth, len(reg))
	var wg sync.WaitGroup
	for i, h := range reg {
		wg.Add(1)
		go func(i int, id domain.Habitat) {
			defer wg.Done()
			out[i] = p.ProbeHabitat(ctx, id)
		}(i, h.ID)
	}
	wg.Wait()
	return out
}

func (p *Prober) ProbeHabitat(ctx context.Context, habitat domain.Habitat) domain.HabitatHealth {
	desc := descriptorFor(habitat)
	health := domain.HabitatHealth{
		Habitat:        habitat,
		LastProbeAt:    time.Now(),
		ConfigPathHint: desc.ConfigHint,
	}

	path, err := exec.LookPath(desc.Binary)
	if err != nil {
		health.Warnings = append(health.Warnings, "Binary not found on PATH.")
		return health
	}
	health.Installed = true
	health.Version = probeVersion(ctx, path)

	switch habitat {
	case domain.HabitatClaude:
		health.Authenticated, health.Warnings = probeClaudeAuth(ctx, path, health.Warnings)
		health.AvailableModels = probeClaudeModels(ctx, path)
	case domain.HabitatCodex:
		health.Authenticated, health.Warnings = probeCodexAuth(ctx, path, health.Warnings)
		health.AvailableModels = probeCodexModels(ctx, path)
	}
	if len(health.AvailableModels) == 0 {
		health.Warnings = append(health.Warnings, "No models discovered from the installed CLI; Estuary will allow manual model entry.")
	}
	return health
}

func descriptorFor(habitat domain.Habitat) habitats.Descriptor {
	for _, item := range habitats.Registry() {
		if item.ID == habitat {
			return item
		}
	}
	return habitats.Descriptor{ID: habitat}
}

func probeVersion(ctx context.Context, path string) string {
	versionCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	raw, err := exec.CommandContext(versionCtx, path, "--version").CombinedOutput()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(raw))
}

func probeClaudeAuth(ctx context.Context, path string, warnings []string) (bool, []string) {
	statusCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	raw, err := exec.CommandContext(statusCtx, path, "auth", "status").CombinedOutput()
	if err != nil {
		warnings = append(warnings, "Authentication status probe failed.")
		return false, warnings
	}
	var payload struct {
		LoggedIn bool `json:"loggedIn"`
	}
	if err := json.Unmarshal(raw, &payload); err == nil {
		return payload.LoggedIn, warnings
	}
	return strings.Contains(strings.ToLower(string(raw)), "logged"), warnings
}

func probeCodexAuth(ctx context.Context, path string, warnings []string) (bool, []string) {
	statusCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	raw, err := exec.CommandContext(statusCtx, path, "login", "status").CombinedOutput()
	if err != nil {
		warnings = append(warnings, "Authentication status probe failed.")
		return false, warnings
	}
	text := strings.ToLower(strings.TrimSpace(string(raw)))
	return strings.Contains(text, "logged in"), warnings
}

func probeClaudeModels(ctx context.Context, path string) []string {
	helpCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	raw, err := exec.CommandContext(helpCtx, path, "--help").CombinedOutput()
	if err != nil {
		return nil
	}
	return uniqueModels(append(extractModelsFromHelp(string(raw), []string{"claude-sonnet-4-6", "sonnet", "opus"}), readClaudeConfigModels()...))
}

func probeCodexModels(ctx context.Context, path string) []string {
	helpCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	raw, err := exec.CommandContext(helpCtx, path, "--help").CombinedOutput()
	if err != nil {
		return readCodexConfigModels()
	}
	models := extractModelsFromHelp(string(raw), []string{"gpt-5", "gpt-5-codex", "codex-mini", "o3", "o4-mini"})
	return uniqueModels(append(models, readCodexConfigModels()...))
}

var modelPattern = regexp.MustCompile(`\b(?:claude-[a-z0-9.-]+|gpt-[a-z0-9.-]+|codex-[a-z0-9.-]+|o[34](?:-[a-z0-9.-]+)?)\b`)

func extractModelsFromHelp(help string, fallback []string) []string {
	found := modelPattern.FindAllString(strings.ToLower(help), -1)
	return uniqueModels(append(found, fallback...))
}

func readClaudeConfigModels() []string {
	return readStringsFromFile(filepath.Join(homeDir(), ".claude", "settings.json"))
}

func readCodexConfigModels() []string {
	return readStringsFromFile(filepath.Join(homeDir(), ".codex", "config.toml"))
}

func readStringsFromFile(path string) []string {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	found := modelPattern.FindAllString(strings.ToLower(string(raw)), -1)
	return uniqueModels(found)
}

func uniqueModels(items []string) []string {
	out := make([]string, 0, len(items))
	for _, item := range items {
		value := strings.TrimSpace(item)
		if value == "" || slices.Contains(out, value) {
			continue
		}
		out = append(out, value)
	}
	return out
}

func homeDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return home
}
