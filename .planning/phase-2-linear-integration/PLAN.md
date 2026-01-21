# Phase 2: Linear Integration - Execution Plan

**Created:** 2026-01-21
**Phase Goal:** Integrate Linear issue tracking with ticket-driven workflows
**Estimated Effort:** 10-12 hours

## Phase Summary

Enable users to link Linear tickets to agent sessions, auto-name workspaces/branches from tickets, and sync session status to Linear.

## Prerequisites

- [x] Phase 1 complete (PTY integration working)
- [x] Research complete (RESEARCH.md)
- [ ] Dependencies installed: `bun add graphql-request graphql @inkjs/ui`

## Key Decision

**Direct GraphQL API with `graphql-request`** instead of `@linear/sdk`:
- SDK is 37.7 MB, graphql-request is ~60 KB (600x smaller)
- We only need 5 operations
- Complete implementation ready in RESEARCH.md

---

## Tasks

### Task 1: Install Dependencies
**Estimate:** 15 minutes
**Priority:** P0 - Blocking

```bash
bun add graphql-request graphql @inkjs/ui
```

**Verification:**
- `bun run build` succeeds
- No type errors

---

### Task 2: Create Linear Service
**Estimate:** 1.5 hours
**Priority:** P0 - Blocking
**Files:** `src/services/linear.ts`

Implement the Linear GraphQL client with:
- `initializeLinear(apiKey)` - Initialize client
- `isLinearInitialized()` - Check if ready
- `validateApiKey(apiKey)` - Test API key validity
- `searchTickets(query, limit?)` - Search issues by title/identifier
- `getTicket(identifier)` - Fetch single issue by ID
- `updateTicketState(identifier, stateId)` - Update issue status
- `createTicket(teamId, title, description?)` - Create new issue
- `getWorkflowStates(teamId)` - Get available statuses

**Reference:** RESEARCH.md lines 817-995 has complete implementation

**Verification:**
- TypeScript compiles with no errors
- Can import and call `validateApiKey` with test key

---

### Task 3: Create Cache Layer
**Estimate:** 1 hour
**Priority:** P1 - Important
**Files:** `src/services/linear-cache.ts`

Simple file-based cache at `~/.agenator/cache/tickets.json`:
- 5-minute TTL for ticket data
- Cache individual tickets by identifier
- Cache search results by query
- Invalidate on update operations

**Verification:**
- Cache file created on first search
- Second search returns cached results (check timestamps)
- Update operation invalidates cache entry

---

### Task 4: Create useLinear Hook
**Estimate:** 1 hour
**Priority:** P0 - Blocking
**Files:** `src/hooks/useLinear.ts`

React hook wrapping Linear service:
- Initialize client from config on mount
- Expose `searchTickets`, `getTicket`, `updateTicketState`, `createTicket`
- Integrate with cache layer (try cache first)
- Return `isInitialized` state

**Verification:**
- Hook initializes correctly with API key from config
- Search returns results (or empty array)

---

### Task 5: Create TicketLinkModal Component
**Estimate:** 2 hours
**Priority:** P0 - Blocking
**Files:** `src/components/TicketLinkModal.tsx`

Modal for searching and linking tickets:
- TextInput for search query (from @inkjs/ui)
- Debounced search (300ms delay)
- Select list for results (from @inkjs/ui)
- Loading state with spinner
- Error display
- Escape to close

**UI Flow:**
1. User presses `[l]`
2. Modal appears with search input
3. User types ticket ID or title
4. Results appear after 300ms debounce
5. User selects with Enter
6. Modal closes, ticket linked to session

**Verification:**
- Modal opens/closes on `l`/Escape
- Search returns results
- Selection triggers onSelect callback

---

### Task 6: Wire Link Action to App
**Estimate:** 1 hour
**Priority:** P0 - Blocking
**Files:** `src/App.tsx`

Connect `[l]` action to TicketLinkModal:
- Track `showTicketModal` state
- Open modal on `l` key
- Close on Escape or selection
- Update session with linked ticket
- Display ticket info in SessionPane header

**Verification:**
- `l` opens modal
- Selecting ticket links it to current session
- Ticket identifier shows in session header

---

### Task 7: Auto-Name Workspace from Ticket
**Estimate:** 1 hour
**Priority:** P1 - Important
**Files:** `src/App.tsx`, `src/services/session.ts`

When creating session with linked ticket:
- Use ticket identifier as workspace name (e.g., `ENG-123`)
- Fall back to prompt if no ticket linked

**Verification:**
- New session with ticket creates workspace named `ENG-123`
- Workspace path shows `~/.agenator/workspaces/ENG-123`

---

### Task 8: Auto-Name Branch from Ticket
**Estimate:** 1 hour
**Priority:** P1 - Important
**Files:** `src/services/workspace.ts`

Generate branch name from ticket:
- Format: `agenator/ENG-123-<slug>`
- Slug from ticket title (lowercase, hyphenated, truncated to 40 chars)
- Example: `agenator/ENG-123-fix-login-redirect`

**Verification:**
- JJ bookmark created with correct name
- `jj bookmark list` shows expected name

---

### Task 9: Status Sync to Linear
**Estimate:** 2 hours
**Priority:** P2 - Nice to Have
**Files:** `src/services/linear.ts`, `src/App.tsx`

Update Linear ticket status on session events:
- Session start -> "In Progress" (type: `started`)
- PR created -> "In Review" (if state exists)
- Session done -> Keep current (don't auto-close)

**Note:** Requires fetching workflow states to find correct state IDs

**Verification:**
- Starting session updates ticket to "In Progress" in Linear
- Check Linear UI to confirm status change

---

### Task 10: Config File Support for API Key
**Estimate:** 30 minutes
**Priority:** P1 - Important
**Files:** `src/config.ts` (create or update)

Load Linear API key from config:
1. Check env: `AGENATOR_LINEAR_API_KEY`
2. Check config: `~/.config/agenator/config.yaml` -> `linear.apiKey`
3. Return undefined if not found (show setup prompt)

**Config format:**
```yaml
linear:
  apiKey: lin_api_xxxxxxxxxxxxxxxxxxxx
  defaultTeam: ENG  # Optional
```

**Verification:**
- API key loaded from config file
- Env variable overrides config file

---

## Task Execution Order

```
Task 1: Install Dependencies (15m)
    |
    v
Task 2: Linear Service (1.5h) ---> Task 3: Cache Layer (1h)
    |                                    |
    v                                    v
Task 4: useLinear Hook (1h) <-----------+
    |
    v
Task 5: TicketLinkModal (2h)
    |
    v
Task 6: Wire to App (1h)
    |
    +--> Task 7: Auto-Name Workspace (1h)
    |
    +--> Task 8: Auto-Name Branch (1h)
    |
    v
Task 10: Config Support (30m)
    |
    v
Task 9: Status Sync (2h) [Optional for MVP]
```

## Definition of Done

- [ ] Can search Linear tickets by ID or title
- [ ] Can link ticket to session via `[l]` action
- [ ] Linked ticket identifier shows in session header
- [ ] New sessions with tickets get auto-named workspaces
- [ ] Branches named `agenator/<ID>-<slug>`
- [ ] API key loads from config file
- [ ] No TypeScript errors
- [ ] Build passes: `bun run build`

## Testing Checklist

1. **API Key Validation**
   - [ ] Invalid key shows error message
   - [ ] Valid key initializes client

2. **Search**
   - [ ] Typing < 2 chars shows nothing
   - [ ] Typing >= 2 chars triggers search
   - [ ] Results appear within 500ms
   - [ ] Empty results show "No tickets found"

3. **Link Flow**
   - [ ] `l` opens modal
   - [ ] Escape closes without selection
   - [ ] Enter on result links ticket
   - [ ] Session header updates with ticket info

4. **Workspace/Branch Naming**
   - [ ] Workspace created at `~/.agenator/workspaces/ENG-123`
   - [ ] Branch named `agenator/ENG-123-slug-here`

## Risks & Mitigations

| Risk | Likelihood | Mitigation |
|------|------------|------------|
| @inkjs/ui incompatible with Ink 5 | Low | Fall back to ink-text-input if needed |
| Rate limiting on search | Medium | Cache aggressively, debounce 300ms |
| GraphQL errors not handled | Medium | Wrap all requests in try/catch |
| Modal focus conflicts | Medium | Check `isOpen` before processing input |

## Files to Create/Modify

**New Files:**
- `src/services/linear.ts` - GraphQL client wrapper
- `src/services/linear-cache.ts` - File-based cache
- `src/hooks/useLinear.ts` - React hook
- `src/components/TicketLinkModal.tsx` - Search/link modal
- `src/config.ts` - Config file loader

**Modified Files:**
- `src/App.tsx` - Wire modal, state management
- `src/services/session.ts` - Auto-naming support
- `src/services/workspace.ts` - Branch naming
- `src/components/SessionPane.tsx` - Display ticket info

---

## Next Steps

1. Run `bun add graphql-request graphql @inkjs/ui`
2. Copy Linear service from RESEARCH.md
3. Build and verify TypeScript compiles
4. Start on Task 2
