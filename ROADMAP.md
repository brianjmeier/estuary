# Agenator Roadmap

> Last updated: January 21, 2026

## Current State Analysis

### What's Built
| Component | Status | Notes |
|-----------|--------|-------|
| Project structure | Done | Bun + Ink (React TUI) + TypeScript |
| Type definitions | Done | Session, Workspace, Config, Actions fully typed |
| Tab-based UI | Done | TabBar with status indicators (!, *, +, x, -) |
| Session pane | Done | Shows ticket info, workspace path, terminal output |
| Status badges | Done | WORKING, NEEDS INPUT, ERROR, PAUSED, DONE, READY |
| Action bar | Done | Keyboard shortcuts [n] [d] [p] [l] [x] [?] |
| Workspace service | Done | JJ workspace create/cleanup/push/diff/status/list |
| Mock data | Done | 3 sample sessions for UI development |

### What's Missing (Gaps)
| Component | Priority | Blocking MVP? |
|-----------|----------|---------------|
| PTY integration (node-pty) | High | Yes |
| Real session management | High | Yes |
| Agent jailing | High | Yes |
| Status detection from output | High | Yes |
| Config loading | Medium | No |
| Linear SDK integration | Medium | No (Phase 2) |
| PR creation workflow | Medium | No (Phase 3) |
| Session persistence | Low | No |
| Desktop notifications | Low | No |

---

## Milestones

### v0.1.0 - MVP: Real Agent Sessions
**Target: 2 weeks**

The MVP should enable running real agent sessions in isolated JJ workspaces with basic status detection.

#### Epic 1: PTY Integration
| Task | Est | Description |
|------|-----|-------------|
| Add node-pty dependency | 1h | Install and configure node-pty for Bun |
| Create PTY service | 4h | Spawn/manage PTY processes, handle I/O |
| Wire PTY to SessionPane | 2h | Stream real terminal output to UI |
| Input forwarding | 2h | Forward keyboard input to active PTY |
| PTY lifecycle management | 2h | Clean shutdown, handle crashes |

#### Epic 2: Session Management
| Task | Est | Description |
|------|-----|-------------|
| Create session flow | 4h | [n] creates new session with workspace |
| Session state machine | 2h | idle -> working -> needs_input -> done |
| Kill session flow | 2h | [x] kills PTY, offers cleanup |
| Session list persistence | 4h | Save/restore sessions.json on disk |

#### Epic 3: JJ Workspace Integration
| Task | Est | Description |
|------|-----|-------------|
| Wire createWorkspace to new session | 2h | New session creates JJ workspace |
| Agent jailing implementation | 4h | PTY spawns with cwd + env isolation |
| Mark done + push flow | 4h | [d] describes revision, creates bookmark |
| Cleanup flow | 2h | [x] on done session cleans workspace |
| Source repo selection | 2h | Prompt or config for source repo |

#### Epic 4: Status Detection
| Task | Est | Description |
|------|-----|-------------|
| Output pattern matcher | 4h | Regex patterns for needs_input/error/done |
| Status update from output | 2h | Auto-update session status on patterns |
| Visual notification on needs_input | 1h | Flash tab, play sound (optional) |

#### Epic 5: Configuration
| Task | Est | Description |
|------|-----|-------------|
| Config file loader | 2h | Load ~/.config/agenator/config.yaml |
| First-run setup wizard | 4h | Prompt for essential config if missing |
| Agent command selection | 1h | Configure opencode vs claude vs custom |

---

### v0.2.0 - Linear Integration
**Target: 2 weeks after MVP**

Integrate Linear for ticket-driven workflows.

#### Epic 6: Linear API Integration
| Task | Est | Description |
|------|-----|-------------|
| Add @linear/sdk dependency | 1h | Install Linear SDK |
| Linear auth flow | 2h | Store/validate API key |
| Ticket search | 4h | Search tickets by query |
| Ticket fetch by ID | 2h | Get ticket details by ENG-123 format |
| Ticket cache | 2h | Cache tickets locally for offline |

#### Epic 7: Linear-Session Linking
| Task | Est | Description |
|------|-----|-------------|
| Link ticket modal | 4h | [l] opens ticket search/link UI |
| Auto-name workspace from ticket | 1h | ENG-123 -> workspace name |
| Auto-name branch from ticket | 2h | ENG-123 -> agenator/ENG-123-slug |
| Display ticket in header | 1h | Show title, priority, assignee |

#### Epic 8: Linear Status Sync
| Task | Est | Description |
|------|-----|-------------|
| Update ticket on session start | 2h | Mark "In Progress" |
| Update ticket on PR created | 2h | Mark "In Review" |
| Create ticket from session | 4h | Quick action to create new ticket |

---

### v0.3.0 - PR Workflow
**Target: 2 weeks after Linear**

Streamline PR creation and review handling.

#### Epic 9: PR Creation
| Task | Est | Description |
|------|-----|-------------|
| Auto-generate PR title | 2h | From ticket ID + title |
| Auto-generate PR description | 4h | From ticket description + jj diff summary |
| Create PR action | 2h | [p] pushes and creates PR via gh |
| PR confirmation UI | 2h | Preview before creating |

#### Epic 10: PR Management
| Task | Est | Description |
|------|-----|-------------|
| PR status polling | 4h | Poll for merged/closed status |
| Display PR comments inline | 4h | Show review comments in UI |
| "Fix comment" action | 4h | Send comment context to agent |
| Auto-cleanup on merge | 2h | Trigger cleanup when PR merged |

---

### v1.0.0 - Production Ready
**Target: 4 weeks after PR Workflow**

Polish, stability, and advanced features.

#### Epic 11: Session Persistence
| Task | Est | Description |
|------|-----|-------------|
| Full session state persistence | 4h | Resume sessions after restart |
| Reconnect to running PTYs | 4h | If PTY still running, reattach |
| Orphan PTY cleanup | 2h | Kill orphaned PTYs on startup |

#### Epic 12: Notifications
| Task | Est | Description |
|------|-----|-------------|
| Desktop notifications | 2h | node-notifier for needs_input |
| Sound notifications | 2h | Optional bell/sound |
| Configurable notification rules | 2h | Per-session or global settings |

#### Epic 13: Advanced Features
| Task | Est | Description |
|------|-----|-------------|
| Session templates | 4h | "new feature" = ticket + workspace + agent |
| Multi-repo support | 4h | Work with multiple source repos |
| Agent handoff | 4h | Transfer context between agents |
| Stacked PRs | 4h | JJ-native stacked diffs workflow |

#### Epic 14: Polish & DX
| Task | Est | Description |
|------|-----|-------------|
| Help modal | 2h | [?] shows all keyboard shortcuts |
| Error handling & recovery | 4h | Graceful error states |
| Loading states | 2h | Spinners for async operations |
| Comprehensive logging | 2h | Debug logs for troubleshooting |
| Test suite | 8h | Unit + integration tests |

---

## Immediate Next Steps (This Week)

### Priority 1: PTY Integration
This is the critical path to MVP. Without real terminal sessions, we can't validate anything else.

1. **Install node-pty**: `bun add node-pty`
2. **Create `src/services/pty.ts`**:
   - `spawnAgent(workspacePath, command)` -> returns PTY handle
   - `writeInput(pty, input)` -> sends input to PTY
   - `onOutput(pty, callback)` -> streams output
   - `killPty(pty)` -> clean shutdown
3. **Create `src/hooks/usePty.ts`**:
   - React hook wrapping PTY service
   - Manages output buffer
   - Handles PTY lifecycle

### Priority 2: Wire Real Sessions
Replace mock data with real session creation.

1. **Update `App.tsx`**:
   - Remove `mockSessions`
   - Add `createSession()` handler for [n]
   - Wire workspace creation + PTY spawn
2. **Create `src/services/session.ts`**:
   - `createSession(opts)` -> creates workspace + spawns PTY
   - `killSession(id)` -> kills PTY, optionally cleans workspace

### Priority 3: Agent Jailing
Ensure agents can't escape their workspace.

1. **Update PTY spawn to set `cwd`** to workspace path
2. **Set environment variables**:
   - `HOME` -> workspace path
   - `PWD` -> workspace path
3. **Test isolation** - verify agent can't `cd ..` outside

---

## Architecture Decisions

### Why node-pty over alternatives?
- Most mature PTY library for Node.js
- Works with Bun (via Node.js compatibility)
- Used by VS Code, Hyper, and other production apps
- Handles edge cases (resize, signal forwarding, etc.)

### Why JJ workspaces over git worktrees?
- True isolation (not just directory separation)
- Automatic rebasing eliminates merge conflicts
- Full operation history with `jj op undo`
- First-class parallel work support
- Simpler mental model for agents

### Why Ink over blessed/terminal-kit?
- React mental model (familiar to most devs)
- Component composition
- Excellent TypeScript support
- Active maintenance
- Works well with Bun

---

## Risk Assessment

| Risk | Likelihood | Impact | Mitigation |
|------|------------|--------|------------|
| node-pty doesn't work with Bun | Medium | High | Test early, fallback to Node if needed |
| JJ workspace perf with many sessions | Low | Medium | Lazy workspace creation, cleanup aggressively |
| Linear API rate limits | Low | Low | Cache aggressively, batch requests |
| PTY output parsing unreliable | Medium | Medium | Use multiple heuristics, allow manual override |
| Session state corruption | Medium | High | Validate state on load, auto-repair |

---

## Open Questions

1. **Agent command**: Should we default to `opencode` or `claude`? Or require explicit config?
2. **Multi-repo**: Should v0.1.0 support multiple source repos, or single repo only?
3. **Workspace location**: Is `~/.agenator/workspaces` the right default?
4. **PTY resize**: How do we handle terminal resize in Ink?
5. **Input mode**: When should input go to agent vs. TUI commands?

---

## Contributing

To pick up a task:
1. Check the [Linear board](https://linear.app) for unassigned issues
2. Assign yourself and move to "In Progress"
3. Create a branch: `agenator/ENG-XXX-short-description`
4. Submit PR when ready

