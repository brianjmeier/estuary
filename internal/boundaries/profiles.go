package boundaries

import (
	"encoding/json"

	"github.com/brianmeier/estuary/internal/domain"
)

const (
	ProfileAskAlways      domain.ProfileID = "ask-always"
	ProfileWorkspaceWrite domain.ProfileID = "workspace-write"
	ProfileReadOnly       domain.ProfileID = "read-only"
	ProfileFullAccess     domain.ProfileID = "full-access"
)

func DefaultProfiles() []domain.BoundaryProfile {
	return []domain.BoundaryProfile{
		{
			ID:                ProfileAskAlways,
			Name:              "Ask Always",
			Description:       "Every risky action requires approval.",
			PolicyLevel:       domain.PolicyLevelStrict,
			FileAccessPolicy:  domain.FileAccessWorkspaceRead,
			CommandExecution:  domain.CommandApprovalRequired,
			NetworkToolPolicy: domain.NetworkApprovalRequired,
			DefaultApproval:   domain.ApprovalAlways,
			HabitatOverrideJSON: mustJSON(map[string]map[string]string{
				"claude": {"permission_mode": "default"},
				"codex":  {"approval_policy": "untrusted", "sandbox_mode": "workspace-write"},
			}),
		},
		{
			ID:                ProfileWorkspaceWrite,
			Name:              "Controlled",
			Description:       "Normal work inside the selected folder, approval for broader access.",
			PolicyLevel:       domain.PolicyLevelBalanced,
			FileAccessPolicy:  domain.FileAccessWorkspaceWrite,
			CommandExecution:  domain.CommandWorkspaceSafe,
			NetworkToolPolicy: domain.NetworkApprovalRequired,
			DefaultApproval:   domain.ApprovalOnRequest,
			HabitatOverrideJSON: mustJSON(map[string]map[string]string{
				"claude": {"permission_mode": "default"},
				"codex":  {"approval_policy": "on-request", "sandbox_mode": "workspace-write"},
			}),
		},
		{
			ID:                ProfileReadOnly,
			Name:              "Read Only",
			Description:       "Inspect and chat only, no writes.",
			PolicyLevel:       domain.PolicyLevelStrict,
			FileAccessPolicy:  domain.FileAccessReadOnly,
			CommandExecution:  domain.CommandReadOnly,
			NetworkToolPolicy: domain.NetworkApprovalRequired,
			DefaultApproval:   domain.ApprovalAlways,
			HabitatOverrideJSON: mustJSON(map[string]map[string]string{
				"claude": {"permission_mode": "default"},
				"codex":  {"approval_policy": "untrusted", "sandbox_mode": "read-only"},
			}),
		},
		{
			ID:                ProfileFullAccess,
			Name:              "Unrestricted",
			Description:       "No approvals. Clearly marked unsafe.",
			PolicyLevel:       domain.PolicyLevelUnsafe,
			FileAccessPolicy:  domain.FileAccessFull,
			CommandExecution:  domain.CommandFullAccess,
			NetworkToolPolicy: domain.NetworkEnabled,
			DefaultApproval:   domain.ApprovalNever,
			Unsafe:            true,
			HabitatOverrideJSON: mustJSON(map[string]map[string]string{
				"claude": {"permission_mode": "bypassPermissions"},
				"codex":  {"approval_policy": "never", "sandbox_mode": "danger-full-access"},
			}),
			CompatibilityNotes: "Use only when you intentionally want native safeguards minimized.",
		},
	}
}

func Resolve(profile domain.BoundaryProfile, habitat domain.Habitat) domain.BoundaryResolution {
	var overrides map[string]map[string]string
	_ = json.Unmarshal([]byte(profile.HabitatOverrideJSON), &overrides)

	settings, ok := overrides[string(habitat)]
	if !ok {
		return domain.BoundaryResolution{
			ProfileID:      profile.ID,
			Habitat:        habitat,
			Compatibility:  domain.BoundaryCompatibilityUnsupported,
			Summary:        "No habitat-native mapping available.",
			NativeSettings: "{}",
		}
	}

	compatibility := domain.BoundaryCompatibilityExact
	summary := "Profile maps cleanly into habitat-native settings."
	if habitat == domain.HabitatClaude {
		switch profile.ID {
		case ProfileAskAlways:
			compatibility = domain.BoundaryCompatibilityApproximated
			summary = "Claude permission mode approximates ask-always behavior."
		case ProfileWorkspaceWrite:
			compatibility = domain.BoundaryCompatibilityApproximated
			summary = "Claude workspace-write behavior is approximated through native permission mode."
		}
	}

	return domain.BoundaryResolution{
		ProfileID:      profile.ID,
		Habitat:        habitat,
		Compatibility:  compatibility,
		Summary:        summary,
		NativeSettings: mustJSON(settings),
	}
}

func mustJSON(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return string(b)
}
