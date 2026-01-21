# Agenator

**AI Agent Orchestration TUI with Linear Integration and JJ Workspace Isolation**

> **TL;DR**: Claude Squad's multi-agent orchestration + Linear ticket integration + JJ workspaces (instead of git worktrees)

## The Problem

When working with multiple AI coding agents (OpenCode, Claude Code, etc.), developers face:

1. **Tab chaos** - Dozens of terminal tabs with no clear way to identify which agent is doing what
2. **Attention fragmentation** - No unified view to know when an agent needs human input vs. working autonomously
3. **Context switching overhead** - Constantly checking different tabs to see progress
4. **No ticket integration** - Manual process to link agent work to Linear tickets
5. **PR workflow friction** - Creating PRs, responding to review comments requires context switching
6. **Workspace conflicts** - Agents can accidentally modify files outside their scope or interfere with each other

## Existing Solutions & Gaps

| Tool | What it does | What's missing for us |
|------|--------------|----------------------|
| **Claude Squad** | tmux-based multi-agent manager, git worktrees | No Linear integration, git worktrees instead of JJ |
| **agentmux** | Paid orchestrator, batch commands, notifications | Closed source, no ticket integration, $49+ |
| **Commander** | Desktop app, multi-agent chat | Electron app (heavy), no Linear/PR workflow |
| **CodeAgentSwarm** | Web-based multi-terminal | Browser-based, no CLI, no ticket integration |

### Why not just use Claude Squad?

[Claude Squad](https://github.com/smtg-ai/claude-squad) is excellent and solves ~80% of the problem. Agenator builds on the same core idea but adds:

1. **Linear Integration** - Link agents to tickets, auto-create branches from ticket IDs, sync ticket status
2. **JJ Workspaces** - Replace git worktrees with Jujutsu workspaces for better isolation, automatic rebasing, and simpler mental model
3. **PR Workflow** - Create PRs with context, view/fix PR comments from within the TUI
4. **Ticket-driven workflow** - Start from a Linear ticket, not from a blank session

**Gap**: None of these tools integrate with project management (Linear) or have a streamlined PR workflow. None use JJ's superior workspace isolation.

## Vision

Agenator is a TUI that:

1. **Orchestrates** multiple OpenCode/Claude Code sessions in a tab-based interface
2. **Isolates** each agent in its own JJ workspace with enforced boundaries
3. **Surfaces status** - clear visual indicators for: working, waiting for input, error, done
4. **Links to Linear** - each agent session is associated with a Linear ticket
5. **Streamlines workflows** - one-click actions for common operations
6. **Cleans up** - automatically removes workspaces when work is complete and PR is merged

---

## Core Feature: JJ Workspace Isolation

Each agent session runs in an isolated [Jujutsu (jj)](https://github.com/martinvonz/jj) workspace. This provides:

### Why JJ over Git Worktrees?

| Feature | Git Worktrees | JJ Workspaces |
|---------|---------------|---------------|
| Setup complexity | Manual branch management | Single command |
| Isolation | Directory-based | Revision-based, true isolation |
| Conflict handling | Manual rebase hell | Automatic rebasing |
| Undo mistakes | Limited (`git reflog`) | Full operation history (`jj op undo`) |
| Parallel work | Possible but awkward | First-class citizen |

### Workspace Lifecycle

```
┌─────────────────────────────────────────────────────────────────┐
│                    WORKSPACE LIFECYCLE                          │
├─────────────────────────────────────────────────────────────────┤
│                                                                 │
│   [Create Session]                                              │
│         │                                                       │
│         ▼                                                       │
│   ┌─────────────┐                                               │
│   │ jj workspace │  Creates: ~/.agenator/workspaces/ENG-123/   │
│   │ add          │  - Isolated working copy                     │
│   │              │  - Own revision off main                     │
│   └─────────────┘                                               │
│         │                                                       │
│         ▼                                                       │
│   ┌─────────────┐                                               │
│   │ Agent runs  │  Agent is JAILED to this directory           │
│   │ in workspace│  - Cannot cd outside                         │
│   │             │  - Cannot push to other branches              │
│   └─────────────┘                                               │
│         │                                                       │
│         ▼                                                       │
│   ┌─────────────┐                                               │
│   │ Mark Done   │  User marks session as "done"                │
│   │ + Create PR │  - jj git push creates branch                 │
│   │             │  - PR opened via gh CLI                       │
│   └─────────────┘                                               │
│         │                                                       │
│         ▼                                                       │
│   ┌─────────────┐                                               │
│   │ PR Merged   │  Cleanup triggered                           │
│   │ → Cleanup   │  - jj workspace forget                        │
│   │             │  - rm -rf workspace directory                 │
│   └─────────────┘                                               │
│                                                                 │
└─────────────────────────────────────────────────────────────────┘
```

### Workspace Directory Structure

```
~/.agenator/
├── config.yaml                    # Global config
├── sessions.json                  # Session state (for persistence)
└── workspaces/
    ├── ENG-123/                   # Workspace for ticket ENG-123
    │   ├── .jj/                   # JJ metadata
    │   ├── .git/                  # Colocated git (for pushing)
    │   └── <project files>        # Working copy
    ├── ENG-456/
    │   └── ...
    └── quick-fix-abc123/          # Workspace without ticket
        └── ...
```

### Agent Jailing

Agents are restricted to their workspace through:

1. **Working directory enforcement**: Agent PTY starts with `cwd` set to workspace
2. **Environment isolation**: `$HOME` and `$PWD` point to workspace
3. **Pre-command hooks**: Block dangerous commands (optional)
4. **JJ configuration**: Workspace-specific `.jj/repo/config.toml`

```typescript
// Workspace jail configuration
interface WorkspaceJail {
  workspacePath: string;           // e.g., ~/.agenator/workspaces/ENG-123
  allowedPaths: string[];          // Read-only access to these paths
  blockedCommands: string[];       // e.g., ['cd ..', 'git push origin main']
  env: Record<string, string>;     // Overridden environment variables
}
```

### Workspace Creation Script

```bash
#!/bin/bash
# create-workspace.sh <session-id> <ticket-id> <source-repo>

SESSION_ID="$1"
TICKET_ID="$2"
SOURCE_REPO="$3"
WORKSPACE_DIR="$HOME/.agenator/workspaces/${TICKET_ID:-$SESSION_ID}"

# Ensure base directory exists
mkdir -p "$HOME/.agenator/workspaces"

# Create workspace from source repo
cd "$SOURCE_REPO"

# Create a new JJ workspace with colocated git
jj workspace add "$WORKSPACE_DIR" --revision main

# Create a new revision for this agent's work
cd "$WORKSPACE_DIR"
jj new -m "agenator: $TICKET_ID - Work in progress"

# Configure workspace-specific settings
cat > .jj/repo/config.toml << EOF
[user]
name = "Agenator Agent"
email = "agent@agenator.dev"

[git]
push-branch-prefix = "agenator/"
EOF

echo "$WORKSPACE_DIR"
```

### Workspace Cleanup Script

```bash
#!/bin/bash
# cleanup-workspace.sh <workspace-path>

WORKSPACE_PATH="$1"
SOURCE_REPO=$(jj workspace root 2>/dev/null)

if [ -z "$SOURCE_REPO" ]; then
  echo "Not a JJ workspace"
  exit 1
fi

# Get workspace name
WORKSPACE_NAME=$(basename "$WORKSPACE_PATH")

# Forget the workspace from JJ
cd "$SOURCE_REPO"
jj workspace forget "$WORKSPACE_NAME" 2>/dev/null || true

# Remove the directory
rm -rf "$WORKSPACE_PATH"

echo "Workspace cleaned up: $WORKSPACE_PATH"
```

---

## Core Features

### Phase 1: Foundation (MVP)
- [ ] Tab-based UI showing multiple agent sessions
- [ ] **JJ workspace creation** for each new session
- [ ] **Agent jailing** - restrict agent to workspace directory
- [ ] Each tab spawns a PTY running `opencode` or `claude` in its workspace
- [ ] Real-time terminal output in a scrollable pane
- [ ] Status detection: `WORKING` | `NEEDS_INPUT` | `ERROR` | `IDLE` | `DONE`
- [ ] Visual badge/indicator when any agent needs input
- [ ] Keyboard navigation between tabs
- [ ] **Mark as Done** action - prepares for PR
- [ ] **Workspace cleanup** when PR is merged

### Phase 2: Linear Integration
- [ ] Link a session to an existing Linear ticket (by ID or search)
- [ ] Display ticket title/description in session header
- [ ] Auto-name workspaces from ticket ID (e.g., `ENG-123`)
- [ ] Auto-name git branches from ticket ID (e.g., `agenator/ENG-123-user-auth`)
- [ ] Quick action: Create new Linear ticket from current context
- [ ] Sync status: mark ticket "In Progress" when agent starts, "In Review" when PR created

### Phase 3: PR Workflow
- [ ] Quick action: Create PR with auto-generated title/description from Linear ticket + jj diff
- [ ] View PR comments inline
- [ ] Quick action: "Fix this comment" - sends comment context to agent
- [ ] PR status indicator per session
- [ ] **Auto-cleanup** when PR is merged (poll or webhook)

### Phase 4: Advanced
- [ ] Session persistence (resume after restart)
- [ ] Notifications (desktop/sound) when agent needs input
- [ ] Session templates (e.g., "new feature" = create ticket + workspace + agent)
- [ ] Multi-repo support
- [ ] Agent handoff (send context from one agent to another)
- [ ] Stacked PRs support (JJ makes this easy)

---

## Architecture

```
+------------------------------------------------------------------+
|                         AGENATOR TUI                              |
+------------------------------------------------------------------+
|  [Tab 1: ENG-123]  [Tab 2: ENG-456]  [Tab 3: Quick Fix]  [+]     |
+------------------------------------------------------------------+
|                                                                   |
|  +-------------------------------------------------------------+  |
|  |  ENG-123: Implement user authentication                     |  |
|  |  Status: NEEDS_INPUT  |  Workspace: ~/.agenator/.../ENG-123 |  |
|  +-------------------------------------------------------------+  |
|  |                                                             |  |
|  |  > opencode                                                 |  |
|  |  I've analyzed the codebase and found the auth module.      |  |
|  |  I can implement JWT-based authentication.                  |  |
|  |                                                             |  |
|  |  Should I proceed with:                                     |  |
|  |  1. JWT tokens with refresh                                 |  |
|  |  2. Session-based auth                                      |  |
|  |  _                                                          |  |
|  |                                                             |  |
|  +-------------------------------------------------------------+  |
|                                                                   |
+------------------------------------------------------------------+
|  [d] Done  [p] Create PR  [l] Link Ticket  [x] Kill  [?] Help    |
+------------------------------------------------------------------+
```

## Technical Approach

### JJ Workspace Management

```typescript
// services/workspace.ts
import { $ } from "bun";

interface WorkspaceOptions {
  sessionId: string;
  ticketId?: string;
  sourceRepo: string;
  baseBranch?: string;
}

export async function createWorkspace(opts: WorkspaceOptions): Promise<string> {
  const workspaceName = opts.ticketId || opts.sessionId;
  const workspacePath = `${process.env.HOME}/.agenator/workspaces/${workspaceName}`;
  
  // Create workspace
  await $`jj workspace add ${workspacePath} --revision ${opts.baseBranch || 'main'}`.cwd(opts.sourceRepo);
  
  // Create new revision for agent work
  await $`jj new -m "agenator: ${workspaceName} - Work in progress"`.cwd(workspacePath);
  
  return workspacePath;
}

export async function cleanupWorkspace(workspacePath: string): Promise<void> {
  const workspaceName = path.basename(workspacePath);
  
  // Get the source repo
  const sourceRepo = await $`jj workspace root`.cwd(workspacePath).text();
  
  // Forget workspace
  await $`jj workspace forget ${workspaceName}`.cwd(sourceRepo.trim()).quiet();
  
  // Remove directory
  await fs.rm(workspacePath, { recursive: true, force: true });
}

export async function pushWorkspace(workspacePath: string, branchName: string): Promise<void> {
  // Describe the current revision
  await $`jj describe -m "Final changes"`.cwd(workspacePath);
  
  // Create a bookmark (JJ's equivalent of a branch)
  await $`jj bookmark create ${branchName}`.cwd(workspacePath);
  
  // Push to remote
  await $`jj git push --bookmark ${branchName}`.cwd(workspacePath);
}
```

### Terminal Management
- Use `node-pty` to spawn pseudo-terminals
- Each agent runs in its own PTY **with cwd set to workspace**
- Parse terminal output to detect status patterns

### Status Detection Heuristics
```typescript
// Pattern matching for OpenCode/Claude Code
const patterns = {
  needsInput: [
    /\?\s*$/,                    // Ends with question mark
    /\[y\/n\]/i,                 // Yes/no prompt
    /press enter/i,             // Waiting for enter
    /choose an option/i,        // Multiple choice
    /waiting for input/i,       // Explicit wait
  ],
  error: [
    /error:/i,
    /failed:/i,
    /exception/i,
  ],
  done: [
    /task completed/i,
    /changes applied/i,
    /done!/i,
  ]
};
```

### Linear Integration
- Use Linear SDK (`@linear/sdk`)
- Store Linear API key in config
- Cache ticket data locally for offline viewing

### Git/PR Integration
- Use JJ's git integration (`jj git push`)
- Use GitHub CLI (`gh`) for PR operations

---

## Tech Stack

- **Runtime**: Bun
- **TUI Framework**: Ink (React for CLI)
- **Terminal**: node-pty
- **VCS**: Jujutsu (jj) with colocated git
- **Linear**: @linear/sdk
- **GitHub**: `gh` CLI

## File Structure

```
src/
├── cli.tsx                 # Entry point
├── App.tsx                 # Main app component
├── types.ts                # Shared types
├── components/
│   ├── TabBar.tsx          # Tab navigation
│   ├── SessionPane.tsx     # Terminal + status display
│   ├── StatusBadge.tsx     # WORKING/NEEDS_INPUT indicator
│   ├── ActionBar.tsx       # Bottom action shortcuts
│   └── index.ts            # Component exports
├── hooks/
│   ├── useSession.ts       # Session management
│   ├── useWorkspace.ts     # JJ workspace operations
│   ├── useLinear.ts        # Linear API
│   └── useStatusDetector.ts # Output parsing
├── services/
│   ├── workspace.ts        # JJ workspace management
│   ├── pty.ts              # PTY management
│   ├── linear.ts           # Linear client
│   ├── github.ts           # GitHub/PR operations
│   └── config.ts           # Configuration
└── scripts/
    ├── create-workspace.sh # Workspace creation script
    └── cleanup-workspace.sh # Workspace cleanup script
```

## Configuration

```yaml
# ~/.config/agenator/config.yaml
linear:
  apiKey: lin_api_xxxxx
  defaultTeam: ENG

github:
  # Uses gh CLI auth by default

agent:
  command: opencode  # or 'claude'
  
workspace:
  baseDir: ~/.agenator/workspaces
  sourceRepo: ~/dev/myproject       # Default repo to create workspaces from
  branchPrefix: agenator/           # Prefix for pushed branches
  cleanupOnMerge: true              # Auto-cleanup when PR merged

ui:
  theme: dark
  notifyOnInput: true
```

---

## Key Differentiators (vs Claude Squad)

| Feature | Claude Squad | Agenator |
|---------|--------------|----------|
| Multi-agent TUI | Yes | Yes |
| Workspace isolation | Git worktrees | JJ workspaces |
| Linear integration | No | Yes - link tickets, sync status |
| Branch naming | Manual | Auto from ticket ID |
| PR creation | Manual (`gh pr create`) | One-key with auto-description |
| PR comment handling | External | View & fix from TUI |
| Workspace cleanup | Manual | Auto on PR merge |
| Undo mistakes | `git reflog` | `jj op undo` |
| Conflict resolution | Manual rebase | Automatic |

### Why These Matter

1. **JJ-native** - Uses Jujutsu for superior workspace isolation (not git worktrees). Automatic rebasing, full operation history, simpler mental model
2. **Agent jailing** - Agents cannot escape their workspace or affect other work
3. **Linear-native** - First-class Linear integration, not an afterthought. Start with a ticket, end with a PR
4. **Status-first UI** - Immediately see which agents need attention
5. **Lifecycle management** - Automatic cleanup when work is complete
6. **Open source** - MIT licensed, community-driven
7. **Lightweight** - Pure TUI, no Electron, runs anywhere

## Success Metrics

- Can manage 5+ concurrent agent sessions without confusion
- Zero cross-contamination between agent workspaces
- Time from "agent done" to "PR created" < 30 seconds
- Never miss an agent waiting for input
- Workspaces are cleaned up within 1 hour of PR merge

---

## Getting Started (Development)

### Prerequisites

```bash
# Install Jujutsu
brew install jj

# Verify installation
jj --version
```

### Development

```bash
# Install dependencies
bun install

# Run in development
bun run dev

# Build
bun run build
```

---

## Roadmap

### v0.1.0 - MVP
- Basic tab UI with sessions
- JJ workspace creation/cleanup
- Agent jailing to workspace
- PTY integration with opencode
- Status detection (needs input vs working)
- "Mark as Done" action
- Manual PR creation via workspace push

### v0.2.0 - Linear Integration
- Create/search/link Linear tickets
- Auto-name workspaces from ticket ID
- Ticket status sync

### v0.3.0 - PR Workflow
- Create PR action (with auto-description)
- View PR comments
- "Fix comment" agent command
- Auto-cleanup on PR merge

### v1.0.0 - Production Ready
- Session persistence
- Notifications
- Polish and stability
- Stacked PRs support
