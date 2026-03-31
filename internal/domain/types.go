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

type SessionRuntimeKind string

const (
	SessionRuntimeKindProviderSession  SessionRuntimeKind = "provider_session"
	SessionRuntimeKindProviderTerminal SessionRuntimeKind = "provider_terminal"
)

type ProviderRuntimeStatus string

const (
	ProviderRuntimeStatusConnecting ProviderRuntimeStatus = "connecting"
	ProviderRuntimeStatusReady      ProviderRuntimeStatus = "ready"
	ProviderRuntimeStatusRunning    ProviderRuntimeStatus = "running"
	ProviderRuntimeStatusDegraded   ProviderRuntimeStatus = "degraded"
	ProviderRuntimeStatusError      ProviderRuntimeStatus = "error"
	ProviderRuntimeStatusClosed     ProviderRuntimeStatus = "closed"
)

type MessageRole string

const (
	MessageRoleUser      MessageRole = "user"
	MessageRoleAssistant MessageRole = "assistant"
	MessageRoleSystem    MessageRole = "system"
	MessageRoleTool      MessageRole = "tool"
	MessageRoleSummary   MessageRole = "summary"
)

type Session struct {
	ID                       string
	Title                    string
	FolderPath               string
	CurrentModel             string
	ModelDescriptor          ModelDescriptor
	CurrentHabitat           Habitat
	RuntimeKind              SessionRuntimeKind
	ActiveProviderSessionID  string
	ProviderStatus           ProviderRuntimeStatus
	NativeSessionID          string
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

type ProviderSessionRef struct {
	ID                       string
	SessionID                string
	Provider                 Habitat
	RuntimeKind              SessionRuntimeKind
	ProviderSessionID        string
	ProviderThreadID         string
	ProviderResumeCursorJSON string
	Status                   ProviderRuntimeStatus
	LastError                string
	StartedAt                time.Time
	UpdatedAt                time.Time
	ClosedAt                 time.Time
}

type ProviderProcessState struct {
	ID                string
	ProviderSessionID string
	SessionID         string
	Provider          Habitat
	RuntimeKind       SessionRuntimeKind
	Transport         string
	Warm              bool
	PID               int
	Connected         bool
	MetadataJSON      string
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

type TerminalFeatureSession struct {
	ID                string
	SessionID         string
	Provider          Habitat
	FeatureKey        string
	TerminalSessionID string
	Status            ProviderRuntimeStatus
	MetadataJSON      string
	CreatedAt         time.Time
	UpdatedAt         time.Time
	ClosedAt          time.Time
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
	VerboseOutput       bool      `json:"verbose_output"`
	AutoAcceptEdits     bool      `json:"auto_accept_edits"`
	StashedPrompt       string    `json:"stashed_prompt"`
}

type TurnEventKind string

const (
	TurnEventStarted       TurnEventKind = "turn.started"
	TurnEventDelta         TurnEventKind = "assistant.delta"
	TurnEventToolStarted   TurnEventKind = "tool.started"
	TurnEventToolOutput    TurnEventKind = "tool.output"
	TurnEventToolFinished  TurnEventKind = "tool.finished"
	TurnEventNotice        TurnEventKind = "notice"
	TurnEventCompleted     TurnEventKind = "turn.completed"
	TurnEventHabitatError  TurnEventKind = "habitat.error"
	TurnEventTaskStarted   TurnEventKind = "task.started"
	TurnEventTaskProgress  TurnEventKind = "task.progress"
	TurnEventTaskComplete  TurnEventKind = "task.completed"
	TurnEventShellStarted  TurnEventKind = "shell.started"
	TurnEventShellOutput   TurnEventKind = "shell.output"
	TurnEventShellComplete TurnEventKind = "shell.completed"
)

type TurnEvent struct {
	Kind            TurnEventKind
	SessionID       string
	TurnID          string
	NativeSessionID string
	Text            string
	ToolName        string
	TaskID          string
	TaskStatus      string
	TaskTitle       string
	TaskDetail      string
	Metadata        map[string]string
	Err             error
}

type ComposeMode string

const (
	ComposeModeChat  ComposeMode = "chat"
	ComposeModeShell ComposeMode = "shell"
)

type TaskSource string

const (
	TaskSourceProvider TaskSource = "provider"
	TaskSourceApp      TaskSource = "app"
)

type SessionTask struct {
	ID             string
	SessionID      string
	ProviderTaskID string
	Source         TaskSource
	Provider       Habitat
	Title          string
	Detail         string
	Status         string
	CreatedAt      time.Time
	UpdatedAt      time.Time
	ClosedAt       time.Time
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

type HabitatHealth struct {
	Habitat          Habitat
	Installed        bool
	Authenticated    bool
	Version          string
	AvailableModels []string
	LastProbeAt     time.Time
	Warnings        []string
	ConfigPathHint  string
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
	FolderPath string
	Model      string
}

type SwitchType string

const (
	SwitchTypeSameProvider  SwitchType = "same_provider"
	SwitchTypeCrossProvider SwitchType = "cross_provider"
	SwitchTypeRestore       SwitchType = "restore"
	SwitchTypeFresh         SwitchType = "fresh"
)

type AttachStrategy string

const (
	AttachStrategyFresh   AttachStrategy = "fresh"
	AttachStrategyResume  AttachStrategy = "resume"
	AttachStrategyHandoff AttachStrategy = "handoff"
)

// HandoffPacket extends MigrationCheckpoint with fields needed for provider switching
// and session restore in the native terminal model.
type HandoffPacket struct {
	MigrationCheckpoint

	RecentWorkSummary string   `json:"recent_work_summary"`
	FileReferences    []string `json:"file_references"`

	SourceModel    string     `json:"source_model"`
	SourceProvider Habitat    `json:"source_provider"`
	TargetModel    string     `json:"target_model"`
	TargetProvider Habitat    `json:"target_provider"`
	SwitchType     SwitchType `json:"switch_type"`
	UserNote       string     `json:"user_note"`
}

// TerminalSession tracks a live PTY-backed native provider session.
type TerminalSession struct {
	ID              string
	SessionID       string
	Provider        Habitat
	PID             int
	AttachStrategy  AttachStrategy
	NativeSessionID string
	HandoffPacketID string
	Status          ProviderRuntimeStatus
	StartedAt       time.Time
	ExitedAt        time.Time
	ExitCode        int
}
