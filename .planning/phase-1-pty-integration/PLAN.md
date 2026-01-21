# Phase 1: PTY Integration - Execution Plan

## Goal
Enable real agent sessions by integrating PTY support, replacing mock data with actual terminal processes.

## Key Discovery
**Bun has native PTY support** via `Bun.spawn()` with `terminal` option (since v1.3.5). No need for node-pty!

## Prerequisites
Your current Bun version is **1.3.2**. You need **1.3.5+** for PTY support.

```bash
# Upgrade Bun first
bun upgrade
```

---

## Tasks

### Task 1: Create PTY Service
**File:** `src/services/pty.ts`
**Est:** 2h
**Dependencies:** None

Create a service that wraps Bun's native PTY API with a clean interface for our app.

```typescript
// Interface design
interface PtyHandle {
  id: string;
  write(input: string): void;
  resize(cols: number, rows: number): void;
  kill(): void;
  onData(callback: (data: string) => void): void;
  onExit(callback: (code: number) => void): void;
}

interface SpawnOptions {
  command: string;
  args?: string[];
  cwd: string;
  env?: Record<string, string>;
  cols?: number;
  rows?: number;
}

// Functions to implement
function spawnPty(opts: SpawnOptions): PtyHandle;
function killPty(handle: PtyHandle): void;
```

**Implementation notes:**
- Use `Bun.spawn()` with `terminal` option
- Wrap in class or object for cleaner API
- Handle cleanup on process exit
- Support resize events

**Acceptance criteria:**
- [ ] Can spawn a PTY running `bash` or `opencode`
- [ ] Can write input to PTY
- [ ] Can receive output from PTY
- [ ] Can kill PTY cleanly
- [ ] PTY respects `cwd` (agent jailing)

---

### Task 2: Create usePty Hook
**File:** `src/hooks/usePty.ts`
**Est:** 1.5h
**Dependencies:** Task 1

React hook that manages PTY lifecycle and integrates with React state.

```typescript
interface UsePtyOptions {
  command: string;
  args?: string[];
  cwd: string;
  env?: Record<string, string>;
  onStatusChange?: (status: SessionStatus) => void;
}

interface UsePtyReturn {
  output: string[];
  write: (input: string) => void;
  kill: () => void;
  isRunning: boolean;
}

function usePty(opts: UsePtyOptions): UsePtyReturn;
```

**Implementation notes:**
- Spawn PTY on mount, kill on unmount
- Buffer output into lines for display
- Limit output buffer size (e.g., 1000 lines)
- Handle PTY exit gracefully

**Acceptance criteria:**
- [ ] PTY spawns when hook mounts
- [ ] PTY kills when hook unmounts
- [ ] Output streams to component state
- [ ] Input can be written via returned function

---

### Task 3: Wire PTY to SessionPane
**File:** `src/components/SessionPane.tsx` (modify)
**Est:** 1.5h
**Dependencies:** Task 2

Replace static `lastOutput` display with live PTY output.

**Changes needed:**
1. Accept PTY hook return as prop (or use hook internally)
2. Display live output instead of `session.lastOutput`
3. Auto-scroll to bottom on new output
4. Handle loading state before PTY ready

**Acceptance criteria:**
- [ ] SessionPane shows real terminal output
- [ ] Output auto-scrolls
- [ ] Loading state while PTY spawns

---

### Task 4: Input Forwarding
**File:** `src/App.tsx` (modify), `src/components/SessionPane.tsx` (modify)
**Est:** 2h
**Dependencies:** Task 3

Forward keyboard input to active PTY when in "input mode".

**Design decisions:**
- **Input mode toggle**: Press `i` to enter input mode (like vim)
- **Exit input mode**: Press `Escape` to return to command mode
- **Visual indicator**: Show "INPUT MODE" badge when active

**Implementation:**
1. Add `inputMode` state to App
2. When `inputMode=true`, forward all keystrokes to active PTY
3. Press `Escape` exits input mode
4. Show visual indicator in ActionBar

**Acceptance criteria:**
- [ ] Press `i` enters input mode
- [ ] In input mode, typing goes to PTY
- [ ] Press `Escape` exits input mode
- [ ] Visual indicator shows current mode

---

### Task 5: PTY Lifecycle Management
**File:** `src/services/pty.ts` (modify), `src/App.tsx` (modify)
**Est:** 1.5h
**Dependencies:** Task 4

Handle PTY crashes, exits, and cleanup.

**Scenarios to handle:**
1. PTY exits normally (agent finished)
2. PTY crashes (unexpected exit)
3. User kills session
4. App closes (cleanup all PTYs)

**Implementation:**
1. Track all active PTYs in a registry
2. On PTY exit, update session status
3. On app exit, kill all PTYs
4. Show error state if PTY crashes unexpectedly

**Acceptance criteria:**
- [ ] Session status updates when PTY exits
- [ ] All PTYs killed when app closes
- [ ] Crashed PTY shows error state
- [ ] No orphan processes left behind

---

### Task 6: Integration Test
**File:** Manual testing
**Est:** 1h
**Dependencies:** All above

End-to-end test of PTY integration.

**Test cases:**
1. Start app, create new session -> PTY spawns
2. Type in input mode -> appears in terminal
3. Run a command (e.g., `ls`) -> output appears
4. Kill session -> PTY terminates
5. Close app -> all PTYs terminate
6. PTY crash -> session shows error

---

## File Changes Summary

| File | Action | Description |
|------|--------|-------------|
| `src/services/pty.ts` | Create | PTY spawn/kill/IO service |
| `src/hooks/usePty.ts` | Create | React hook for PTY |
| `src/components/SessionPane.tsx` | Modify | Wire live PTY output |
| `src/App.tsx` | Modify | Input mode, PTY lifecycle |
| `src/components/ActionBar.tsx` | Modify | Input mode indicator |
| `src/types.ts` | Modify | Add PtyHandle type |

---

## Open Questions

1. **Output buffer size**: How many lines to keep? (Propose: 1000)
2. **Input mode UX**: Is `i`/`Escape` the right pattern? Alternative: always forward input when session pane focused
3. **Terminal size**: How to get Ink's terminal dimensions for PTY?

---

## Execution Order

```
Task 1: PTY Service (2h)
    ↓
Task 2: usePty Hook (1.5h)
    ↓
Task 3: Wire to SessionPane (1.5h)
    ↓
Task 4: Input Forwarding (2h)
    ↓
Task 5: Lifecycle Management (1.5h)
    ↓
Task 6: Integration Test (1h)

Total: ~9.5h
```

---

## Success Criteria

Phase is complete when:
- [ ] Can spawn a real `opencode` or `bash` session
- [ ] Can see live output in the TUI
- [ ] Can type input to the agent
- [ ] Sessions clean up properly
- [ ] No orphan processes

