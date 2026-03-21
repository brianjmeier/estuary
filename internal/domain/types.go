package domain

import "time"

type Habitat string

const (
	HabitatClaude Habitat = "claude"
	HabitatCodex  Habitat = "codex"
)

type SessionStatus string

const (
	SessionStatusIdle   SessionStatus = "idle"
	SessionStatusActive SessionStatus = "active"
	SessionStatusError  SessionStatus = "error"
)

type MessageRole string

const (
	MessageRoleUser      MessageRole = "user"
	MessageRoleAssistant MessageRole = "assistant"
	MessageRoleSystem    MessageRole = "system"
	MessageRoleTool      MessageRole = "tool"
	MessageRoleSummary   MessageRole = "summary"
)

type BoundaryCompatibility string

const (
	BoundaryCompatibilityExact        BoundaryCompatibility = "exact"
	BoundaryCompatibilityApproximated BoundaryCompatibility = "approximated"
	BoundaryCompatibilityUnsupported  BoundaryCompatibility = "unsupported"
)

type ProfileID string

type PolicyLevel string

const (
	PolicyLevelStrict   PolicyLevel = "strict"
	PolicyLevelBalanced PolicyLevel = "balanced"
	PolicyLevelUnsafe   PolicyLevel = "unsafe"
)

type FileAccessPolicy string

const (
	FileAccessWorkspaceRead  FileAccessPolicy = "workspace-read"
	FileAccessWorkspaceWrite FileAccessPolicy = "workspace-write"
	FileAccessReadOnly       FileAccessPolicy = "read-only"
	FileAccessFull           FileAccessPolicy = "full-access"
)

type CommandPolicy string

const (
	CommandApprovalRequired CommandPolicy = "approval-required"
	CommandWorkspaceSafe    CommandPolicy = "workspace-safe"
	CommandReadOnly         CommandPolicy = "read-only"
	CommandFullAccess       CommandPolicy = "full-access"
)

type NetworkPolicy string

const (
	NetworkApprovalRequired NetworkPolicy = "approval-required"
	NetworkEnabled          NetworkPolicy = "enabled"
)

type ApprovalBehavior string

const (
	ApprovalAlways    ApprovalBehavior = "always"
	ApprovalOnRequest ApprovalBehavior = "on-request"
	ApprovalNever     ApprovalBehavior = "never"
)

type Session struct {
	ID                       string
	Title                    string
	FolderPath               string
	CurrentModel             string
	ModelDescriptor          ModelDescriptor
	CurrentHabitat           Habitat
	NativeSessionID          string
	BoundaryProfile          ProfileID
	ResolvedBoundarySettings string
	Status                   SessionStatus
	MigrationGeneration      int
	CreatedAt                time.Time
	UpdatedAt                time.Time
	LastOpenedAt             time.Time
}

type Message struct {
	ID        string
	SessionID string
	TurnID    string
	Role      MessageRole
	Content   string
	Source    string
	CreatedAt time.Time
}

type RuntimeEvent struct {
	ID        string
	SessionID string
	EventType string
	Payload   string
	CreatedAt time.Time
}

type SessionRuntimeState struct {
	Active              bool      `json:"active"`
	ResumeExplicit      bool      `json:"resume_explicit"`
	LastResumeStatus    string    `json:"last_resume_status"`
	LastResumeAttemptAt time.Time `json:"last_resume_attempt_at"`
	PendingContinuation string    `json:"pending_continuation"`
	PendingCheckpointID string    `json:"pending_checkpoint_id"`
	ActiveTraitIDs      []string  `json:"active_trait_ids"`
	FirstRunCompleted   bool      `json:"first_run_completed"`
}

type TurnEventKind string

const (
	TurnEventStarted      TurnEventKind = "turn.started"
	TurnEventDelta        TurnEventKind = "assistant.delta"
	TurnEventToolStarted  TurnEventKind = "tool.started"
	TurnEventToolOutput   TurnEventKind = "tool.output"
	TurnEventToolFinished TurnEventKind = "tool.finished"
	TurnEventNotice       TurnEventKind = "notice"
	TurnEventCompleted    TurnEventKind = "turn.completed"
	TurnEventHabitatError TurnEventKind = "habitat.error"
)

type TurnEvent struct {
	Kind            TurnEventKind
	SessionID       string
	NativeSessionID string
	Text            string
	ToolName        string
	Metadata        map[string]string
	Err             error
}

type MigrationCheckpoint struct {
	ID                  string            `json:"id"`
	SessionID           string            `json:"session_id"`
	ActiveObjective     string            `json:"active_objective"`
	ImportantDecisions  []string          `json:"important_decisions"`
	FolderPath          string            `json:"folder_path"`
	CurrentModel        string            `json:"current_model"`
	CurrentHabitat      Habitat           `json:"current_habitat"`
	ConversationSummary string            `json:"conversation_summary"`
	OpenTasks           []string          `json:"open_tasks"`
	ActiveTraits        []string          `json:"active_traits"`
	RecentToolOutputs   []string          `json:"recent_tool_outputs"`
	HabitatNotes        map[string]string `json:"habitat_notes"`
	CreatedAt           time.Time         `json:"created_at"`
}

type TraitType string

const (
	TraitTypeCommand TraitType = "command"
	TraitTypeSkill   TraitType = "skill"
	TraitTypeTool    TraitType = "tool"
)

const (
	TraitScopeShared         = "shared"
	TraitSyncBootstrap       = "bootstrap"
	TraitDispatchInjected    = "injected-context"
	TraitDispatchUnsupported = "unsupported"
)

type Trait struct {
	ID             string
	Type           TraitType
	Name           string
	Description    string
	Scope          string
	CanonicalDef   string
	SupportsClaude bool
	SupportsCodex  bool
	SyncMode       string
	DispatchMode   string
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

type BoundaryProfile struct {
	ID                  ProfileID
	Name                string
	Description         string
	PolicyLevel         PolicyLevel
	FileAccessPolicy    FileAccessPolicy
	CommandExecution    CommandPolicy
	NetworkToolPolicy   NetworkPolicy
	DefaultApproval     ApprovalBehavior
	HabitatOverrideJSON string
	CompatibilityNotes  string
	Unsafe              bool
}

type BoundaryResolution struct {
	ProfileID      ProfileID
	Habitat        Habitat
	Compatibility  BoundaryCompatibility
	Summary        string
	NativeSettings string
}

type HabitatHealth struct {
	Habitat          Habitat
	Installed        bool
	Authenticated    bool
	Version          string
	AvailableModels  []string
	LastProbeAt      time.Time
	Warnings         []string
	ConfigPathHint   string
	BoundaryBehavior string
}

type ModelDescriptor struct {
	ID          string
	Label       string
	Habitat     Habitat
	SpeciesNote string
}

type AppSettings struct {
	Theme string
}

type SessionDraft struct {
	FolderPath      string
	Model           string
	BoundaryProfile string
}
