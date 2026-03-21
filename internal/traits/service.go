package traits

import (
	"context"
	"fmt"
	"strings"

	"github.com/brianmeier/estuary/internal/domain"
	"github.com/brianmeier/estuary/internal/store"
)

type Service struct {
	store *store.Store
}

func NewService(st *store.Store) *Service {
	return &Service{store: st}
}

func (s *Service) List(ctx context.Context) ([]domain.Trait, error) {
	return s.store.ListTraits(ctx)
}

func (s *Service) Save(ctx context.Context, trait domain.Trait) (domain.Trait, error) {
	return s.store.UpsertTrait(ctx, normalizeTrait(trait))
}

func (s *Service) Delete(ctx context.Context, traitID string) error {
	return s.store.DeleteTrait(ctx, traitID)
}

func (s *Service) ActiveForSession(ctx context.Context, session domain.Session) ([]domain.Trait, error) {
	items, err := s.store.ListTraits(ctx)
	if err != nil {
		return nil, err
	}
	var out []domain.Trait
	for _, item := range items {
		if supportsHabitat(item, session.CurrentHabitat) {
			out = append(out, item)
		}
	}
	return out, nil
}

func (s *Service) ResolveCommand(ctx context.Context, session domain.Session, input string) (domain.Trait, string, error) {
	text := strings.TrimSpace(input)
	if !strings.HasPrefix(text, "/") {
		return domain.Trait{}, input, nil
	}
	parts := strings.Fields(strings.TrimPrefix(text, "/"))
	if len(parts) == 0 {
		return domain.Trait{}, input, nil
	}
	items, err := s.ActiveForSession(ctx, session)
	if err != nil {
		return domain.Trait{}, input, err
	}
	for _, item := range items {
		if item.Type != domain.TraitTypeCommand {
			continue
		}
		if strings.EqualFold(item.Name, parts[0]) {
			if item.DispatchMode == domain.TraitDispatchUnsupported {
				return item, "", fmt.Errorf("trait %q is unsupported on %s", item.Name, session.CurrentHabitat)
			}
			remainder := strings.TrimSpace(strings.TrimPrefix(text, "/"+parts[0]))
			return item, injectTrait(item, remainder), nil
		}
	}
	return domain.Trait{}, input, nil
}

func normalizeTrait(trait domain.Trait) domain.Trait {
	trait.Name = strings.TrimSpace(trait.Name)
	trait.Description = strings.TrimSpace(trait.Description)
	trait.Scope = fallback(trait.Scope, domain.TraitScopeShared)
	trait.SyncMode = fallback(trait.SyncMode, domain.TraitSyncBootstrap)
	trait.DispatchMode = fallback(trait.DispatchMode, domain.TraitDispatchInjected)
	return trait
}

func supportsHabitat(trait domain.Trait, habitat domain.Habitat) bool {
	switch habitat {
	case domain.HabitatClaude:
		return trait.SupportsClaude
	case domain.HabitatCodex:
		return trait.SupportsCodex
	default:
		return false
	}
}

func injectTrait(trait domain.Trait, remainder string) string {
	if remainder == "" {
		return fmt.Sprintf("Invoke trait %s:\n%s", trait.Name, trait.CanonicalDef)
	}
	return fmt.Sprintf("Invoke trait %s with user request %q.\nTrait definition:\n%s", trait.Name, remainder, trait.CanonicalDef)
}

func fallback(value, defaultValue string) string {
	if strings.TrimSpace(value) == "" {
		return defaultValue
	}
	return value
}
