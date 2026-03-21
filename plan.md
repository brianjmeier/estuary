PLEASE IMPLEMENT THIS PLAN:
# Estuary vNext: Go TUI for Unified Claude Code and Codex Sessions

## Summary

Build `Estuary` as a local Go TUI that centralizes Claude Code and Codex usage behind one multi-session chat interface.

The app is intentionally narrow:

- no git clone management
- no repo registry
- no branch workflow
- no app-managed workspaces
- no ticket or PR workflow
- no app-owned system prompt layer

Estuary’s job is to coordinate, persist, and bridge native provider sessions without redefining them.

Core responsibilities:

- multi-session tabs
- common chat UI
- user-picked local folders as session roots
- model-first provider routing
- persisted and resumable sessions
- app-managed continuity for model/provider switching via `Migration`
- shared commands, skills, and tools managed once in `Traits`
- provider-specific health and key config managed through `Habitat Settings`
- app-owned permission profiles translated into provider-native settings via `Boundaries`

## Product Decisions Locked

- App name: `Estuary`
- Language/runtime: Go
- Shell: TUI
- Session model: multi-session tabs
- Session root: user-picked local folder
- Multiple live sessions per folder: allowed, with warning
- User chooses model, app maps to habitat
- Model catalog: detected from installed habitats
- UI contract: common chat only
- Persistence: local transcript + metadata persistence, resume when possible
- Habitat switching: allowed inside one Estuary session via app-managed `Migration`
- Shared commands/skills/tools: core v1 feature under `Traits`
- Shared capability source of truth: Estuary-owned registry
- Provider-specific setup: health + key settings only under `Habitat Settings`
- Backend integration mode: mixed by habitat, using the cleanest implementation per backend
- No app-owned system prompt
- No hidden app persona or cross-provider harness override prompt
- `Boundaries` are app-owned and explicitly configured by the user
- Estuary translates `Boundaries` into habitat-native settings
- Use `Species` as supporting product language for models, but keep `Model` as the primary technical label in detailed UI
- Every implemented feature must update `FEATURES.md`
- `README.md` must reference `FEATURES.md` as the up-to-date implemented feature index
- Development environment must be provisioned with Nix flakes
- Project setup must make `go` and required tooling available through Nix, not ad hoc local installation
- Any implementation instructions that rely on Nix must assume `source /nix/var/nix/profiles/default/etc/profile.d/nix-daemon.sh` before using Nix commands

## Naming System

### Primary Product Vocabulary

- App: `Estuary`
- Sessions: `Sessions`
- Providers/backends: `Habitats`
- Models: `Models`
- Shared setup: `Traits`
- Provider-specific setup: `Habitat Settings`
- Continuity switch: `Migration`
- Health: `Ecosystem`
- Permissions: `Boundaries`

### Supporting Language

Use `Species` as framing copy, not as the only primary UI label.

Examples:

- “Choose a model and Estuary will route it to the right habitat.”
- “This species is native to the Claude habitat.”
- “Migration continues this session in a new habitat.”
- “Traits are shared across habitats when compatible.”

### UI Labeling Rule

Use standard labels where precision matters:

- `Model`
- `Session`
- `Provider` only in technical/debug surfaces if needed

Use metaphorical labels in navigation and product framing:

- `Habitats`
- `Traits`
- `Migration`
- `Ecosystem`
- `Boundaries`

## Documentation Rule

### Feature Inventory

`FEATURES.md` is the canonical inventory of implemented product behavior.

Rules:

- every time a feature is implemented, expanded, materially changed, or removed, update `FEATURES.md` in the same change
- `FEATURES.md` should describe shipped behavior, not aspirational roadmap items
- `FEATURES.md` should be organized by user-visible capability areas
- each feature entry should indicate current status plainly:
  - implemented
  - partial
  - experimental
- implementation work is not complete unless `FEATURES.md` is updated accordingly

### README Linkage

`README.md` must reference `FEATURES.md` prominently as the current feature index.

Rules:

- `README.md` should contain a clear link to `FEATURES.md`
- `README.md` should not become the full feature matrix
- if `README.md` summarizes features, it must point readers to `FEATURES.md` for the authoritative current list

### Delivery Requirement

For every future implementation plan or feature task, include:

- code change
- verification
- `FEATURES.md` update
- `README.md` update if the reference is missing or stale

## Development Environment

### Tooling Strategy

Use Nix flakes for project-local development tooling.

Goals:

- reproducible Go toolchain
- reproducible formatter/linter/test tooling
- no dependence on whatever Go version happens to be installed globally
- easier onboarding for future contributors and future sessions

### Required Setup Artifacts

Repository should include:

- `flake.nix`
- `flake.lock`
- optionally `.envrc` if later paired with `direnv`, but not required for v1
- updated `README.md` setup instructions describing how to enter the dev environment

### Nix Shell Requirements

The flake dev shell should provide at minimum:

- `go`
- `gofmt`
- `gcc` or required C toolchain if needed by Go dependencies
- `git`
- `sqlite`
- any additional CLI tooling required for development, such as:
  - `golangci-lint` if adopted
  - `just` if adopted
  - `delve` if adopted

Provider CLIs like `claude` and `codex` are not required to be installed through Nix for v1.
They remain external runtime dependencies detected by the app.

### Nix Usage Rule

All implementation/setup documentation must assume:

```bash
source /nix/var/nix/profiles/default/etc/profile.d/nix-daemon.sh
```

before running Nix commands if Nix is not already in the shell environment.

### Dev Workflow Rule

Preferred project entry commands should become:

- `nix develop`
- then Go project commands inside that shell

or, if using `nix develop -c` style, document that explicitly.

## Design Language

### Theme Statement

Estuary should feel like a coastal control room for intelligent systems:
calm, literate, habitat-aware, and operationally trustworthy.

### Visual Principles

- Different habitats, one shoreline
- Technical clarity first, metaphor second
- Strong hierarchy with generous breathing room
- Calm operational feel, not flashy “AI tool” theatrics
- Shared product identity across all habitats
- Restraint in color, motion, and ornament

### Light and Dark Theme Strategy

Estuary must support first-class light and dark themes.

Rules:

- Light and dark are two expressions of the same estuarial design system
- Same hierarchy, spacing, labeling, and habitat semantics in both themes
- Dark mode must not collapse into generic neon hacker-terminal aesthetics
- Light mode must not collapse into flat white enterprise blankness
- All components use semantic tokens, not hardcoded colors

### Light Theme Direction

Light theme should evoke:

- fog
- sand
- silt
- reeds
- tidepool water

Characteristics:

- warm neutral surfaces
- restrained cool-water accents
- readable contrast without sterile white
- habitat colors as subtle annotations

### Dark Theme Direction

Dark theme should evoke:

- deep inlet water
- wet stone
- marsh at dusk
- peat and shoreline shadows

Characteristics:

- layered dark surfaces, not pure black
- muted blue-green depth tones
- warm mineral accents for balance
- calm operational feel, not sci-fi neon

### Dark Mode Rules

- backgrounds use softened charcoal-blue / peat / deep-water tones, not flat black
- borders are low-contrast and sparse
- active states use water-blue or reed-green, not electric cyan
- warnings use muted amber
- danger uses oxidized rust/red, not bright alarm red
- habitat accents remain localized and subtle
- readability in chat and status surfaces takes priority over visual mood

### Color Semantics

Core semantic accents:

- water: focus, active selection, migration flow
- reed: safe progress, confirmation, ready states
- clay: warmth, provider annotation, grounded emphasis
- amber: warning, approximation, caution
- rust: danger, destructive actions, unsafe full access

Habitat tint guidance:

- Claude habitat: warm mineral / clay / amber tint
- Codex habitat: cool water / steel-blue tint

Habitat colors must not dominate the whole UI.

### Typography

- one serious monospace for chat, logs, code-like detail
- one readable UI face for labels, tabs, panels
- stable vertical rhythm across panes
- prioritize legibility over novelty
- avoid default “developer app” sameness when the environment allows stronger choices

### Layout Rules

- Left: Sessions
- Center: Chat
- Right: Context panel
  - Model
  - Habitat
  - Boundaries
  - Traits in effect
  - Ecosystem warnings
- Modals for:
  - Migration
  - Habitat Settings
  - Traits
  - Boundaries
- Setup/configuration should not overload the main chat pane

### Motion and Interaction

- session switching: quick and quiet
- streaming output: steady incremental reveal
- migration: one clear transition state with explicit checkpointing
- warnings and errors: precise, noticeable, never theatrical

### Anti-Patterns To Ban

- generic hacker-terminal aesthetic
- random purple gradients
- excessive box nesting
- too many borders
- metaphor overload in primary labels
- provider colors taking over the full interface
- hidden unsafe states
- cute wildlife branding that makes the app unserious
- dark mode as pure black + neon accents
- light mode as blank white with no atmosphere

### Terminology Usage Rules

- Use `Model` in controls and technical detail
- Use `species` in helper copy and onboarding language
- Use `Habitats` in navigation and setup
- Use `Traits` for shared commands/skills/tools
- Use `Migration` only for cross-model/provider continuity
- Use `Boundaries` consistently for permission profiles
- Use `Ecosystem` for health/readiness only

### Theme Token Requirements

Define semantic theme tokens at minimum for:

- `bg.canvas`
- `bg.surface`
- `bg.panel`
- `fg.primary`
- `fg.muted`
- `border.soft`
- `accent.water`
- `accent.reed`
- `accent.clay`
- `status.warning`
- `status.danger`
- `status.success`
- `habitat.claude`
- `habitat.codex`

Both light and dark themes must derive from these same semantic tokens.

## Architecture

### Top-Level Components

1. `cmd/estuary`
   - app entrypoint
   - bootstraps config, DB, and TUI runtime

2. `internal/app`
   - root Bubble Tea model
   - routing, hotkeys, modal state, focus management

3. `internal/sessions`
   - create/open/close sessions
   - session switching
   - persistence and resume orchestration
   - folder conflict warnings

4. `internal/habitats`
   - habitat registry
   - model discovery
   - per-habitat session runtimes
   - habitat capability metadata
   - habitat-native boundary translation

5. `internal/habitats/claude`
   - Claude Code integration
   - structured session/turn handling where available
   - Claude-specific migration/bootstrap logic
   - Claude boundary/config projection

6. `internal/habitats/codex`
   - Codex integration
   - structured or app-server-driven session/turn handling where practical
   - Codex-specific migration/bootstrap logic
   - Codex boundary/config projection

7. `internal/chat`
   - app-owned normalized chat timeline
   - assistant/user/system/tool message model
   - streaming aggregation

8. `internal/migration`
   - habitat switching handoff
   - session compaction checkpoints
   - continuation prompt generation

9. `internal/traits`
   - Estuary-owned registry for shared commands, skills, and tools
   - habitat compatibility metadata
   - habitat projection/sync helpers

10. `internal/habitatconfig`
   - habitat health checks
   - key settings read/write
   - habitat setup summaries

11. `internal/boundaries`
   - app-owned boundary profile model
   - translation into habitat-native settings
   - compatibility and mismatch reporting

12. `internal/store`
   - SQLite migrations
   - persistence layer for sessions, messages, checkpoints, traits, settings

13. `internal/prereq`
   - verifies `claude` and `codex`
   - checks auth/readiness
   - probes model lists

## Core Runtime Model

### Session

Stores:

- session id
- title
- current folder path
- current model
- current habitat
- provider-native session id if available
- selected boundary profile
- resolved habitat-native boundary settings
- status
- created_at / updated_at / last_opened_at
- migration generation number

### Message

Stores:

- message id
- session id
- turn id
- role: user | assistant | system | tool | summary
- content blocks
- source: user | provider | migration
- timestamps

### Runtime Event

Stores events such as:

- session.created
- session.resumed
- session.folder-bound
- model.changed
- habitat.changed
- boundaries.changed
- turn.started
- assistant.delta
- tool.started
- tool.output
- tool.finished
- migration.checkpoint.created
- habitat.error
- session.closed

### Trait

Stores:

- trait id
- type: command | skill | tool
- name
- description
- scope: global | folder | session
- canonical definition
- supports_claude
- supports_codex
- sync_mode: native | bootstrap-only | unsupported
- dispatch_mode: provider-native | injected-context | unsupported
- timestamps

### Boundary Profile

Stores:

- profile id
- name
- description
- policy level
- file access policy
- command execution policy
- network/tool policy if configurable
- default approval behavior
- per-habitat override mappings if needed
- compatibility notes

## User Experience

### Main Layout

- left pane: session tabs/list
- center pane: active chat timeline
- bottom pane: composer / action hints
- right pane or modal: session context, model, habitat, folder, status, boundaries, trait shortcuts

### Core Flows

#### New Session

1. User starts new session.
2. User picks a local folder.
3. App warns if another live session already targets that folder.
4. App probes available models from installed habitats.
5. User picks model.
6. App maps model to habitat.
7. User selects a boundary profile or accepts the default.
8. App translates that profile into habitat-native settings.
9. App starts habitat session.
10. Estuary opens a common chat timeline for that session.

#### Send Message

1. User submits message in common chat UI.
2. App persists user message immediately.
3. App forwards the turn to the current habitat runtime.
4. Habitat output is normalized into streaming timeline items.
5. Final assistant/tool output is persisted.

#### Migration

1. User changes model in the session.
2. App determines whether the habitat changes too.
3. App creates a migration checkpoint from Estuary-owned session state.
4. App starts or resumes a habitat session for the new model/habitat.
5. App injects minimal continuation context.
6. Chat continues in the same Estuary session timeline, with a migration event recorded.

## Common Chat Contract

The visible UI is habitat-neutral.

The common chat must support:

- streaming assistant output
- tool activity rows
- system notices
- session status indicators
- migration notices
- user-authored messages
- app-authored summaries/checkpoints
- approval-needed prompts when the active boundary profile requires them

The UI does not expose raw terminals in v1.

If a habitat has behavior that does not map cleanly, represent it as:

- a system event
- a habitat-specific detail row
- or a structured fallback block in the timeline

## Harness Rule: No Estuary System Prompt

This is a hard rule.

Estuary must not introduce:

- a canonical system prompt
- a hidden app persona
- a cross-habitat “meta harness” prompt that overrides native behavior

The app should preserve each model inside its native habitat.

Allowed text injection is limited to:

- user-authored messages
- explicit trait payloads the user enabled
- minimal migration summaries when switching habitats
- operational context strictly required to continue a session

Estuary coordinates the habitats. It does not replace them.

## Backend Integration Strategy

### Principle

Use the best implementation mode per habitat, but preserve one Estuary-owned session model.

That means:

- Claude and Codex do not need identical internal adapters.
- Estuary owns the user-facing session/timeline contract.
- Habitat-specific details stay behind habitat runtime interfaces.

### Claude Habitat

Use Claude Code’s cleaner structured interfaces where available for:

- streaming output
- session continuation
- model selection
- boundary projection
- trait integration where safe

### Codex Habitat

Use the cleanest stable path available, such as:

- structured execution mode
- app-server-like integration if it materially improves session handling
- session continuation through Estuary’s migration layer
- native approval/sandbox setting projection

### Why Mixed-By-Habitat

This matches the `t3code` lesson you liked:

- common UX
- habitat-specific internals
- avoid forcing a lowest-common-denominator backend layer

## Migration and Continuity

### Ownership

Migration is app-owned.

Habitats may have native resume/continue features, but Estuary always writes its own canonical checkpoint.

### Trigger Rules

Create migration checkpoints:

- before habitat/model switch
- before risky resume paths
- when session history grows beyond heuristic thresholds
- on manual user action

### Checkpoint Contents

- active objective
- important prior decisions
- current folder path
- current model/habitat
- rolling conversation summary
- open tasks/questions
- active traits in effect
- recent notable tool outputs
- habitat-specific notes worth preserving

### Continuation Model

Estuary does not attempt to share opaque hidden habitat state.
It preserves app-owned working context and rehydrates the next habitat session from that.

## Traits

### Scope

This is in v1, not deferred.

### Canonical Model

Estuary owns the canonical `Traits` registry.

Habitats receive projections of these traits when possible, but habitat-native storage is not the source of truth.

### Categories

#### Commands
Reusable executable or provider-routed actions the user can invoke from Estuary.

#### Skills
Reusable instruction bundles / role overlays / workflow prompts.

#### Tools
Named capability definitions that may map to habitat-native tool config, injected instructions, or unsupported states.

### Trait Behavior

Each trait must define:

- name
- type
- description
- scope
- canonical payload
- compatibility metadata
- how it gets projected per habitat
- whether it is invokable from the active session

### UX Requirements

- `Traits` setup screen for commands, skills, tools
- Explicit compatibility labels:
  - Shared
  - Claude-only
  - Codex-only
  - Partial
- Clear failure states when a trait cannot be projected into the current habitat

## Boundaries

### Principle

`Boundaries` are configured by the user in Estuary, then translated into each habitat’s native settings.

Estuary does not inherit `t3code`’s binary naming or defaults.

### Boundary Profiles

V1 should support these canonical profiles:

- `Ask Always`
  - every risky action requires approval
- `Workspace Write`
  - allow normal work in the selected folder, escalate for broader access
- `Read Only`
  - inspect and chat only, no writes
- `Full Access`
  - no approvals, clearly marked unsafe

### Translation Layer

Estuary maps these profiles to habitat-native settings such as:

- Claude Code
  - permission mode
  - dangerous skip permissions toggle
  - allowed tools if needed
  - additional directories if configured

- Codex
  - approval policy
  - sandbox mode
  - writable/additional dirs
  - provider-specific safety knobs

### Compatibility Rules

For every session, Estuary must show whether the applied mapping is:

- `Exact`
- `Approximated`
- `Unsupported`

If a habitat cannot represent the requested boundary profile exactly, the user must see that before the session starts.

### Setup Requirements

Setup UI must allow:

- default boundary profile
- per-habitat default overrides
- optional per-folder overrides later
- clear unsafe labeling for `Full Access`

## Habitat Settings

### Scope in v1

Only health + key settings.

Do not attempt a full native config editor in v1.

### Per-Habitat Setup Screen Must Show

- installed / missing
- authenticated / unauthenticated
- available models
- key paths or config file pointers
- a small set of important toggles/settings Estuary can manage safely
- default boundary mapping behavior
- last successful probe timestamp

### What Stays Out of Scope

- exhaustive editing of every habitat-native setting
- pretending Claude and Codex configs share one common schema

## Ecosystem

### Ecosystem View

`Ecosystem` is the health and readiness surface.

It should show:

- installed habitats
- authentication state
- available models
- last probe times
- version info if useful
- trait compatibility warnings
- boundary translation warnings

## Persistence

### SQLite Tables

Minimum tables:

- `sessions`
- `session_runtime`
- `messages`
- `events`
- `migration_checkpoints`
- `traits`
- `habitat_settings`
- `ecosystem_snapshots`
- `boundary_profiles`
- `session_boundary_resolutions`
- `app_settings`

### Filesystem Layout

```text
~/.estuary/
  data/
    estuary.db
  logs/
  config/
```

Project repository root must also contain:

- `README.md`
- `FEATURES.md`
- `flake.nix`
- `flake.lock`

`README.md` references `FEATURES.md`.
Runtime data under `~/.estuary/` must not be treated as the source of truth for repository documentation.

No app-managed workspaces or clones.

## TUI Technology

### UI Stack

- Go
- Bubble Tea
- Lip Gloss
- Bubbles components where useful

### Why TUI Works

The interface is focused on:

- tabs
- one chat pane
- settings/setup modals
- migration flow
- boundaries
- traits
- ecosystem state

That fits a TUI well.

## Milestones

### Milestone 1: Core Shell

Goal: boot the Go TUI with persistence, ecosystem checks, and reproducible dev setup.

Deliver:

- Go project scaffold
- Bubble Tea shell
- SQLite migrations
- Ecosystem screen
- app settings
- Boundary profile scaffolding
- session list scaffolding
- initial `FEATURES.md`
- `README.md` reference to `FEATURES.md`
- `flake.nix` and `flake.lock`
- documented Nix-based dev shell setup

### Milestone 2: Session Runtime

Goal: create and persist real habitat-backed sessions.

Deliver:

- new session flow
- folder picker/input
- duplicate-folder warning
- model discovery
- habitat mapping
- boundary profile selection
- habitat-native boundary translation
- habitat session startup
- streaming assistant output into common timeline
- update `FEATURES.md`

### Milestone 3: Persistence and Resume

Goal: make sessions durable.

Deliver:

- persisted transcript and metadata
- resume/reopen flow
- habitat-native resume when available
- graceful degraded restore when only transcript is recoverable
- update `FEATURES.md`

### Milestone 4: Migration

Goal: make one Estuary session portable across models/habitats.

Deliver:

- migration checkpoint service
- model/habitat switch action
- continuation prompt generation with minimal operational context
- timeline notices for migration and rehydration
- update `FEATURES.md`

### Milestone 5: Traits

Goal: bring commands, skills, and tools into the app core.

Deliver:

- trait storage
- Traits setup UI
- compatibility metadata
- projection into habitats where possible
- session-level invocation hooks
- update `FEATURES.md`

### Milestone 6: Boundaries and Habitat Settings Polish

Goal: make safety and setup explicit.

Deliver:

- Boundary profile management UI
- exact/approximated/unsupported mapping indicators
- unsafe full-access labeling
- Habitat Settings views for key settings
- first-run setup flow
- update `FEATURES.md`

### Milestone 7: Daily-Use Polish

Goal: make the tool usable every day.

Deliver:

- status indicators
- better error states
- search/filter for sessions
- diagnostics/log view
- help screen and keyboard shortcuts
- update `FEATURES.md`

## Public Interfaces and Types

Important Go service contracts:

- `HabitatRegistry`
- `HabitatRuntime`
- `SessionService`
- `ChatService`
- `MigrationService`
- `TraitService`
- `HabitatConfigService`
- `BoundaryService`

Important domain types:

- `Session`
- `Message`
- `RuntimeEvent`
- `MigrationCheckpoint`
- `Trait`
- `HabitatHealth`
- `ModelDescriptor`
- `BoundaryProfile`
- `BoundaryResolution`

## Acceptance Criteria

A v1 build is acceptable when:

- user can open multiple sessions in tabs
- each session is rooted in a user-selected local folder
- app warns when multiple live sessions target the same folder
- user picks a model and Estuary maps the habitat
- user picks a boundary profile and sees the resolved habitat-native mapping
- session runs through a common chat UI
- assistant output streams into the timeline
- sessions persist across restarts
- app resumes or gracefully restores sessions when possible
- user can migrate across habitats/models inside one Estuary session
- traits can be defined once in Estuary
- Habitat Settings show health and key settings for Claude and Codex
- Estuary does not inject a hidden system prompt layer
- both light and dark themes preserve the same estuarial design language
- `FEATURES.md` reflects the actually implemented feature set
- `README.md` links readers to `FEATURES.md`
- `nix develop` provides the documented Go/tooling environment after sourcing the Nix daemon profile if needed

## Testing and Scenarios

### Unit Tests

- habitat mapping from model selection
- folder conflict detection
- session state transitions
- migration checkpoint generation
- boundary profile translation
- boundary compatibility resolution
- message/event persistence
- trait compatibility rules
- habitat health parsing
- habitat-specific runtime adapters
- theme token usage rules and semantic color mapping
- documentation completeness checks for `FEATURES.md` presence where practical

### Integration Tests

- create Claude-backed session from chosen folder
- create Codex-backed session from chosen folder
- stream output into normalized chat timeline
- persist and restore session transcript
- migrate habitat within one session using migration checkpoint
- define trait and project it to supported habitat runtime
- Ecosystem reflects actual habitat availability
- boundary profile maps correctly into habitat-native startup settings
- light and dark themes preserve layout hierarchy and semantic status colors
- repository docs include `FEATURES.md` and `README.md` references it
- Nix dev shell provides the expected Go/tooling commands

### Failure Cases

- Claude missing
- Codex missing
- habitat unauthenticated
- selected folder missing or inaccessible
- selected model disappears between probe and launch
- requested boundary profile cannot be represented cleanly
- session resume partially fails
- migration bootstrap fails
- trait unsupported by chosen habitat
- dark theme contrast regression
- habitat tint overwhelms readability in either theme
- implemented feature not reflected in `FEATURES.md`
- Nix shell missing required Go/tooling dependencies

## Assumptions and Defaults

- App is local-first and single-user.
- Estuary does not manage repos or working directories beyond binding a session to an existing folder.
- Habitats are installed separately by the user.
- Estuary owns the common chat/session model.
- Estuary owns the canonical Traits registry.
- Habitat-specific config remains habitat-specific, with only key settings exposed in v1.
- Mixed backend internals are acceptable as long as the user-facing session model stays unified.
- Estuary preserves native habitat behavior instead of replacing it with a common system prompt.
- User-configured Boundaries are canonical, and habitat settings are derived projections.
- Light and dark themes share one semantic token system and one design identity.
- `FEATURES.md` is the canonical implemented-feature ledger and must be maintained continuously.
- Nix flakes are the canonical development environment definition for this repository.