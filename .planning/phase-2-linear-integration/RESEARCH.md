# Phase 2: Linear Integration - Research

**Researched:** 2026-01-21
**Domain:** Linear GraphQL API, Ink TUI Components, Caching
**Confidence:** HIGH

## Summary

This research covers the integration of Linear's issue tracking system into the Agenator TUI. After evaluating both the `@linear/sdk` and direct GraphQL API approaches, **we recommend using the GraphQL API directly with `graphql-request`**.

**Why not @linear/sdk?**
- Unpacked size is **37.7 MB** - excessive for a CLI tool (current build is 1.91 MB)
- We only need ~5 operations (search, get, update, create issues + get workflow states)
- SDK is auto-generated wrapper around GraphQL anyway
- Simpler dependencies = better Bun compatibility

**Why direct GraphQL with graphql-request?**
- Bundle size: ~60 KB gzipped vs 37 MB
- Full flexibility to write exactly the queries we need
- Easier to debug and understand
- We can build a thin typed abstraction (~100 lines of code)

For the TUI components, `@inkjs/ui` provides ready-to-use `TextInput` and `Select` components that are well-suited for the ticket search/link modal.

**Primary recommendation:** Use `graphql-request` for API interactions, build a thin typed Linear client abstraction, use `@inkjs/ui` for modal components, implement a simple file-based cache with 5-minute TTL for offline support.

## Standard Stack

The established libraries/tools for this domain:

### Core
| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| graphql-request | ^7.0.0 | Lightweight GraphQL client | 60KB gzipped, minimal deps, TypeScript support |
| @inkjs/ui | latest | TUI input/select components | By Ink author, integrates seamlessly |

### Supporting
| Library | Version | Purpose | When to Use |
|---------|---------|---------|-------------|
| graphql | ^16.0.0 | GraphQL query parsing (peer dep) | Required by graphql-request |

### Alternatives Considered
| Instead of | Could Use | Tradeoff |
|------------|-----------|----------|
| graphql-request | @linear/sdk | Official but 37.7 MB unpacked - way too heavy |
| graphql-request | Native fetch | Even lighter but lose gql template tag and error handling |
| @inkjs/ui | ink-select-input | Older, less maintained, fewer features |

### SDK vs Direct API Analysis

| Factor | @linear/sdk | graphql-request |
|--------|-------------|-----------------|
| **Unpacked size** | 37.7 MB | 320 KB |
| **Gzipped size** | ~5 MB | ~60 KB |
| **Type safety** | Built-in | Manual (we define types) |
| **Maintenance** | Very active (v71 released today) | Stable, mature |
| **Operations needed** | ~5 | ~5 |
| **Bun compatibility** | Unknown | High (simple deps) |

**Decision:** Direct GraphQL with graphql-request. The 600x size difference is not justified for 5 operations.

**Installation:**
```bash
bun add graphql-request graphql @inkjs/ui
```

## Architecture Patterns

### Recommended Project Structure
```
src/
├── services/
│   ├── linear.ts           # Linear API client wrapper
│   └── linear-cache.ts     # Ticket caching layer
├── hooks/
│   ├── useLinear.ts        # React hook for Linear operations
│   └── useDebounce.ts      # Debounce hook for search
├── components/
│   └── TicketLinkModal.tsx # Modal for searching/linking tickets
└── types.ts                # Already has LinearTicket type
```

### Pattern 1: Linear GraphQL Client Abstraction
**What:** Thin wrapper around graphql-request with typed queries and error handling
**When to use:** All Linear API interactions
**Example:**
```typescript
// src/services/linear.ts
import { GraphQLClient, gql } from "graphql-request";
import type { LinearTicket } from "../types.js";

const LINEAR_API = "https://api.linear.app/graphql";

let client: GraphQLClient | null = null;

export function initializeLinear(apiKey: string): void {
  client = new GraphQLClient(LINEAR_API, {
    headers: { Authorization: apiKey },
  });
}

export function isLinearInitialized(): boolean {
  return client !== null;
}

// GraphQL Queries
const VIEWER_QUERY = gql`
  query Viewer {
    viewer { id, name, email }
  }
`;

const SEARCH_ISSUES_QUERY = gql`
  query SearchIssues($query: String!, $teamKey: String, $first: Int!) {
    issues(
      first: $first
      filter: {
        or: [
          { title: { containsIgnoreCase: $query } }
          { identifier: { containsIgnoreCase: $query } }
        ]
        team: { key: { eq: $teamKey } }
      }
    ) {
      nodes {
        id
        identifier
        title
        description
        url
        priority
        state { id, name, type }
        assignee { id, name }
      }
    }
  }
`;

const GET_ISSUE_QUERY = gql`
  query GetIssue($identifier: String!) {
    issue(id: $identifier) {
      id
      identifier
      title
      description
      url
      priority
      state { id, name, type }
      assignee { id, name }
    }
  }
`;

// Response types (what GraphQL returns)
interface IssueNode {
  id: string;
  identifier: string;
  title: string;
  description: string | null;
  url: string;
  priority: number;
  state: { id: string; name: string; type: string } | null;
  assignee: { id: string; name: string } | null;
}

function mapIssueToTicket(issue: IssueNode): LinearTicket {
  return {
    id: issue.id,
    identifier: issue.identifier,
    title: issue.title,
    description: issue.description ?? undefined,
    url: issue.url,
    priority: issue.priority,
    state: issue.state?.name ?? "Unknown",
    assignee: issue.assignee?.name ?? undefined,
  };
}

export async function validateApiKey(apiKey: string): Promise<boolean> {
  try {
    const testClient = new GraphQLClient(LINEAR_API, {
      headers: { Authorization: apiKey },
    });
    await testClient.request(VIEWER_QUERY);
    return true;
  } catch (error: any) {
    if (error?.response?.status === 401) {
      return false;
    }
    throw error;
  }
}

export async function searchTickets(
  query: string, 
  teamKey?: string
): Promise<LinearTicket[]> {
  if (!client) throw new Error("Linear client not initialized");
  
  const response = await client.request<{ issues: { nodes: IssueNode[] } }>(
    SEARCH_ISSUES_QUERY,
    { query, teamKey, first: 20 }
  );
  
  return response.issues.nodes.map(mapIssueToTicket);
}

export async function getTicketByIdentifier(
  identifier: string
): Promise<LinearTicket | null> {
  if (!client) throw new Error("Linear client not initialized");
  
  try {
    const response = await client.request<{ issue: IssueNode | null }>(
      GET_ISSUE_QUERY,
      { identifier }
    );
    return response.issue ? mapIssueToTicket(response.issue) : null;
  } catch {
    return null;
  }
}
```

### Pattern 2: Caching Layer
**What:** Simple file-based cache with TTL for offline support and reduced API calls
**When to use:** All ticket data that doesn't need real-time updates
**Example:**
```typescript
// src/services/linear-cache.ts
import * as fs from "fs/promises";
import * as path from "path";
import type { LinearTicket } from "../types.js";

interface CacheEntry<T> {
  data: T;
  timestamp: number;
}

const CACHE_DIR = `${process.env.HOME}/.agenator/cache`;
const CACHE_FILE = "tickets.json";
const DEFAULT_TTL = 5 * 60 * 1000; // 5 minutes

interface TicketCache {
  tickets: Record<string, CacheEntry<LinearTicket>>;
  searchResults: Record<string, CacheEntry<string[]>>; // query -> ticket IDs
}

async function loadCache(): Promise<TicketCache> {
  try {
    const data = await fs.readFile(path.join(CACHE_DIR, CACHE_FILE), "utf-8");
    return JSON.parse(data);
  } catch {
    return { tickets: {}, searchResults: {} };
  }
}

async function saveCache(cache: TicketCache): Promise<void> {
  await fs.mkdir(CACHE_DIR, { recursive: true });
  await fs.writeFile(
    path.join(CACHE_DIR, CACHE_FILE),
    JSON.stringify(cache, null, 2)
  );
}

export async function getCachedTicket(identifier: string): Promise<LinearTicket | null> {
  const cache = await loadCache();
  const entry = cache.tickets[identifier];
  
  if (entry && Date.now() - entry.timestamp < DEFAULT_TTL) {
    return entry.data;
  }
  return null;
}

export async function cacheTicket(ticket: LinearTicket): Promise<void> {
  const cache = await loadCache();
  cache.tickets[ticket.identifier] = {
    data: ticket,
    timestamp: Date.now(),
  };
  await saveCache(cache);
}

export async function cacheSearchResults(query: string, tickets: LinearTicket[]): Promise<void> {
  const cache = await loadCache();
  
  // Cache individual tickets
  for (const ticket of tickets) {
    cache.tickets[ticket.identifier] = {
      data: ticket,
      timestamp: Date.now(),
    };
  }
  
  // Cache search results as list of IDs
  cache.searchResults[query.toLowerCase()] = {
    data: tickets.map(t => t.identifier),
    timestamp: Date.now(),
  };
  
  await saveCache(cache);
}

export async function getCachedSearchResults(query: string): Promise<LinearTicket[] | null> {
  const cache = await loadCache();
  const entry = cache.searchResults[query.toLowerCase()];
  
  if (entry && Date.now() - entry.timestamp < DEFAULT_TTL) {
    const tickets: LinearTicket[] = [];
    for (const id of entry.data) {
      const ticketEntry = cache.tickets[id];
      if (ticketEntry) {
        tickets.push(ticketEntry.data);
      }
    }
    return tickets.length > 0 ? tickets : null;
  }
  return null;
}
```

### Pattern 3: Modal Component Pattern for Ink
**What:** Use useFocusManager to create modal overlay behavior
**When to use:** Link ticket modal, create ticket modal
**Example:**
```typescript
// src/components/TicketLinkModal.tsx
import { useState, useEffect, useCallback } from "react";
import { Box, Text, useFocusManager, useInput } from "ink";
import { TextInput, Select } from "@inkjs/ui";
import { Spinner } from "ink-spinner";
import type { LinearTicket } from "../types.js";

interface TicketLinkModalProps {
  isOpen: boolean;
  onClose: () => void;
  onSelect: (ticket: LinearTicket) => void;
  searchTickets: (query: string) => Promise<LinearTicket[]>;
}

export function TicketLinkModal({ 
  isOpen, 
  onClose, 
  onSelect,
  searchTickets 
}: TicketLinkModalProps) {
  const { disableFocus, enableFocus } = useFocusManager();
  const [query, setQuery] = useState("");
  const [results, setResults] = useState<LinearTicket[]>([]);
  const [isLoading, setIsLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [mode, setMode] = useState<"search" | "select">("search");

  // Disable parent focus when modal is open
  useEffect(() => {
    if (isOpen) {
      // Modal manages its own focus
    }
  }, [isOpen]);

  // Handle escape to close
  useInput((input, key) => {
    if (!isOpen) return;
    if (key.escape) {
      onClose();
    }
  });

  // Debounced search effect
  useEffect(() => {
    if (!isOpen || query.length < 2) {
      setResults([]);
      return;
    }

    const timer = setTimeout(async () => {
      setIsLoading(true);
      setError(null);
      try {
        const tickets = await searchTickets(query);
        setResults(tickets);
        if (tickets.length > 0) {
          setMode("select");
        }
      } catch (err) {
        setError(err instanceof Error ? err.message : "Search failed");
      } finally {
        setIsLoading(false);
      }
    }, 300);

    return () => clearTimeout(timer);
  }, [query, isOpen, searchTickets]);

  if (!isOpen) return null;

  return (
    <Box
      flexDirection="column"
      borderStyle="round"
      borderColor="cyan"
      paddingX={1}
      paddingY={1}
    >
      <Text bold color="cyan">Link Linear Ticket</Text>
      
      <Box marginTop={1}>
        <Text color="gray">Search: </Text>
        <TextInput
          placeholder="Type ticket ID or title..."
          value={query}
          onChange={setQuery}
        />
      </Box>

      {isLoading && (
        <Box marginTop={1}>
          <Spinner type="dots" />
          <Text color="gray"> Searching...</Text>
        </Box>
      )}

      {error && (
        <Box marginTop={1}>
          <Text color="red">{error}</Text>
        </Box>
      )}

      {!isLoading && results.length > 0 && (
        <Box marginTop={1} flexDirection="column">
          <Select
            options={results.map((ticket) => ({
              label: `${ticket.identifier}: ${ticket.title}`,
              value: ticket.identifier,
            }))}
            onChange={(value) => {
              const ticket = results.find((t) => t.identifier === value);
              if (ticket) onSelect(ticket);
            }}
          />
        </Box>
      )}

      {!isLoading && query.length >= 2 && results.length === 0 && (
        <Box marginTop={1}>
          <Text color="gray">No tickets found</Text>
        </Box>
      )}

      <Box marginTop={1}>
        <Text color="gray">[Esc] Cancel</Text>
      </Box>
    </Box>
  );
}
```

### Pattern 4: useLinear Hook
**What:** React hook that wraps Linear service with React lifecycle
**When to use:** Components that need Linear data
**Example:**
```typescript
// src/hooks/useLinear.ts
import { useState, useEffect, useCallback } from "react";
import type { LinearTicket } from "../types.js";
import * as linear from "../services/linear.js";
import * as cache from "../services/linear-cache.js";

interface UseLinearResult {
  isInitialized: boolean;
  searchTickets: (query: string) => Promise<LinearTicket[]>;
  getTicket: (identifier: string) => Promise<LinearTicket | null>;
  updateTicketStatus: (identifier: string, stateId: string) => Promise<void>;
  createTicket: (opts: CreateTicketOpts) => Promise<LinearTicket>;
}

interface CreateTicketOpts {
  teamId: string;
  title: string;
  description?: string;
  priority?: number;
}

export function useLinear(apiKey?: string): UseLinearResult {
  const [isInitialized, setIsInitialized] = useState(false);

  useEffect(() => {
    if (apiKey) {
      linear.initializeLinear(apiKey);
      setIsInitialized(true);
    }
  }, [apiKey]);

  const searchTickets = useCallback(async (query: string): Promise<LinearTicket[]> => {
    // Try cache first
    const cached = await cache.getCachedSearchResults(query);
    if (cached) return cached;

    // Fetch from API
    const tickets = await linear.searchTickets(query);
    await cache.cacheSearchResults(query, tickets);
    return tickets;
  }, []);

  const getTicket = useCallback(async (identifier: string): Promise<LinearTicket | null> => {
    // Try cache first
    const cached = await cache.getCachedTicket(identifier);
    if (cached) return cached;

    // Fetch from API
    const ticket = await linear.getTicketByIdentifier(identifier);
    if (ticket) {
      await cache.cacheTicket(ticket);
    }
    return ticket;
  }, []);

  const updateTicketStatus = useCallback(async (
    identifier: string, 
    stateId: string
  ): Promise<void> => {
    await linear.updateTicketStatus(identifier, stateId);
    // Invalidate cache for this ticket
    await cache.invalidateCachedTicket(identifier);
  }, []);

  const createTicket = useCallback(async (opts: CreateTicketOpts): Promise<LinearTicket> => {
    const ticket = await linear.createTicket(opts);
    await cache.cacheTicket(ticket);
    return ticket;
  }, []);

  return {
    isInitialized,
    searchTickets,
    getTicket,
    updateTicketStatus,
    createTicket,
  };
}
```

### Anti-Patterns to Avoid
- **Using @linear/sdk for simple use cases:** The 37.7 MB SDK is overkill for 5 operations. Direct GraphQL is 600x smaller.
- **Polling for updates:** Don't poll Linear API for changes. The 5-minute cache TTL is sufficient for this use case. If real-time updates are needed later, use webhooks.
- **Fetching all issues:** Never fetch all issues and filter client-side. Always use GraphQL filters with `first: N` limit.
- **Storing API key in code:** Always use config file or environment variable.
- **Ignoring rate limits:** Linear has rate limits. The cache helps, but also handle 429 responses with exponential backoff.
- **Over-fetching fields:** Only request the fields you need in GraphQL queries. Don't copy-paste large queries.

## Don't Hand-Roll

Problems that look simple but have existing solutions:

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| Text input with autocomplete | Custom input component | `@inkjs/ui` TextInput | Handles edge cases, keyboard nav |
| Select/dropdown list | Custom list component | `@inkjs/ui` Select | Scroll, highlight, accessibility |
| GraphQL client | Raw fetch | `graphql-request` | gql tag, error handling, TypeScript |
| Debounce | setInterval logic | useDebounce hook | Cleanup, edge cases |

**What TO hand-roll:**
| Problem | Build It | Why |
|---------|----------|-----|
| Linear API abstraction | ~100 line service | SDK is 37.7 MB, we need 5 operations |
| Type definitions | Extend existing types.ts | Already have LinearTicket, just add response types |

**Key insight:** Ink UI components handle terminal edge cases (cursor, escape sequences, scrolling) that are tedious to implement correctly. But API clients for limited operations are worth building yourself.

## Common Pitfalls

### Pitfall 1: GraphQL Error Handling
**What goes wrong:** Unhandled GraphQL errors crash the app
**Why it happens:** graphql-request throws on non-200 responses
**How to avoid:** Wrap requests in try/catch and check for specific error types:
```typescript
try {
  const result = await client.request(QUERY, variables);
} catch (error: any) {
  if (error?.response?.status === 401) {
    // Invalid API key
  } else if (error?.response?.status === 429) {
    // Rate limited - back off and retry
  } else if (error?.response?.errors) {
    // GraphQL validation errors
    console.error(error.response.errors);
  }
  throw error;
}
```
**Warning signs:** Unhandled promise rejections, generic error messages

### Pitfall 2: Search Filter Syntax
**What goes wrong:** Search returns no results or errors
**Why it happens:** Using wrong filter syntax (e.g., `contains` vs `containsIgnoreCase`)
**How to avoid:** Use `containsIgnoreCase` for case-insensitive search:
```graphql
filter: {
  or: [
    { title: { containsIgnoreCase: $query } }
    { identifier: { containsIgnoreCase: $query } }
  ]
}
```
**Warning signs:** "ENG-123" not matching "eng-123", empty results for valid queries

### Pitfall 3: Modal Focus Management in Ink
**What goes wrong:** Parent component captures keyboard input while modal is open
**Why it happens:** Ink's useInput hook fires for all components by default
**How to avoid:** Use useFocusManager to disable parent focus when modal is open, or check `isOpen` state before processing input in parent
**Warning signs:** Modal and parent both responding to same keystrokes

### Pitfall 4: Cache Invalidation
**What goes wrong:** Showing stale ticket data after status changes
**Why it happens:** Cache TTL not expired, or not invalidating on writes
**How to avoid:** Invalidate specific cache entries on update operations:
```typescript
export async function updateTicketStatus(identifier: string, stateId: string): Promise<void> {
  await client.updateIssue(identifier, { stateId });
  // Invalidate cache for this ticket
  await invalidateCachedTicket(identifier);
}
```
**Warning signs:** UI shows old status after updating via Linear app

### Pitfall 5: Environment Variable vs Config File
**What goes wrong:** API key not found in production/different environments
**Why it happens:** Only checking one location
**How to avoid:** Check multiple sources in order:
```typescript
function getApiKey(): string | undefined {
  // 1. Environment variable (highest priority)
  if (process.env.AGENATOR_LINEAR_API_KEY) {
    return process.env.AGENATOR_LINEAR_API_KEY;
  }
  // 2. Config file
  const config = loadConfig();
  return config.linear?.apiKey;
}
```
**Warning signs:** Works locally but not in CI/production

## Code Examples

Direct GraphQL queries for all Linear operations we need:

### Initialize Client
```typescript
import { GraphQLClient } from "graphql-request";

const client = new GraphQLClient("https://api.linear.app/graphql", {
  headers: { Authorization: "lin_api_xxxxxxxxxxxxx" },
});
```

### Search Issues by Title/Identifier
```graphql
query SearchIssues($query: String!, $first: Int!) {
  issues(
    first: $first
    filter: {
      or: [
        { title: { containsIgnoreCase: $query } }
        { identifier: { containsIgnoreCase: $query } }
      ]
    }
  ) {
    nodes {
      id
      identifier
      title
      url
      priority
      state { id, name, type }
      assignee { name }
    }
  }
}
```

### Fetch Issue by Identifier
```graphql
query GetIssue($id: String!) {
  issue(id: $id) {
    id
    identifier
    title
    description
    url
    priority
    state { id, name, type }
    assignee { name }
  }
}
```
Note: The `id` parameter accepts both UUID and identifier format (e.g., "ENG-123").

### Update Issue Status
```graphql
mutation UpdateIssueState($issueId: String!, $stateId: String!) {
  issueUpdate(id: $issueId, input: { stateId: $stateId }) {
    success
    issue {
      id
      identifier
      state { name }
    }
  }
}
```

### Create New Issue
```graphql
mutation CreateIssue($teamId: String!, $title: String!, $description: String, $priority: Int) {
  issueCreate(input: {
    teamId: $teamId
    title: $title
    description: $description
    priority: $priority
  }) {
    success
    issue {
      id
      identifier
      title
      url
    }
  }
}
```

### Fetch Workflow States for Team
```graphql
query GetWorkflowStates($teamId: String!) {
  team(id: $teamId) {
    id
    key
    states {
      nodes {
        id
        name
        type
        position
      }
    }
  }
}
```
State types: `backlog`, `unstarted`, `started`, `completed`, `canceled`

### Get Current User (for API key validation)
```graphql
query Viewer {
  viewer {
    id
    name
    email
  }
}
```

### Ink UI TextInput with Autocomplete
```typescript
// Source: Context7 /vadimdemedes/ink-ui docs
import { TextInput } from '@inkjs/ui';

<TextInput
  placeholder="Search tickets..."
  suggestions={recentTickets.map(t => t.identifier)}
  onChange={setQuery}
  onSubmit={handleSearch}
/>
```

### Ink UI Select Component
```typescript
// Source: Context7 /vadimdemedes/ink-ui docs
import { Select } from '@inkjs/ui';

<Select
  options={tickets.map(ticket => ({
    label: `${ticket.identifier}: ${ticket.title}`,
    value: ticket.id,
  }))}
  visibleOptionCount={5}
  onChange={handleSelect}
/>
```

### Focus Management
```typescript
// Source: Context7 /vadimdemedes/ink docs
import { useFocusManager, useInput } from 'ink';

function Modal({ isOpen, onClose }) {
  const { disableFocus, enableFocus } = useFocusManager();
  
  useEffect(() => {
    if (isOpen) {
      // Could disable parent focus here
    }
    return () => enableFocus();
  }, [isOpen]);
  
  useInput((input, key) => {
    if (!isOpen) return;
    if (key.escape) onClose();
  });
}
```

## Complete Linear Service Implementation

Reference implementation for `src/services/linear.ts` (~120 lines):

```typescript
// src/services/linear.ts
import { GraphQLClient, gql } from "graphql-request";
import type { LinearTicket } from "../types.js";

const LINEAR_API = "https://api.linear.app/graphql";

let client: GraphQLClient | null = null;

// ============ Initialization ============

export function initializeLinear(apiKey: string): void {
  client = new GraphQLClient(LINEAR_API, {
    headers: { Authorization: apiKey },
  });
}

export function isLinearInitialized(): boolean {
  return client !== null;
}

export async function validateApiKey(apiKey: string): Promise<boolean> {
  try {
    const testClient = new GraphQLClient(LINEAR_API, {
      headers: { Authorization: apiKey },
    });
    await testClient.request(gql`query { viewer { id } }`);
    return true;
  } catch {
    return false;
  }
}

// ============ Types ============

interface IssueNode {
  id: string;
  identifier: string;
  title: string;
  description: string | null;
  url: string;
  priority: number;
  state: { id: string; name: string; type: string } | null;
  assignee: { id: string; name: string } | null;
}

interface WorkflowState {
  id: string;
  name: string;
  type: string;
  position: number;
}

// ============ Queries ============

const SEARCH_ISSUES = gql`
  query SearchIssues($query: String!, $first: Int!) {
    issues(
      first: $first
      filter: {
        or: [
          { title: { containsIgnoreCase: $query } }
          { identifier: { containsIgnoreCase: $query } }
        ]
      }
      orderBy: updatedAt
    ) {
      nodes {
        id identifier title description url priority
        state { id name type }
        assignee { id name }
      }
    }
  }
`;

const GET_ISSUE = gql`
  query GetIssue($id: String!) {
    issue(id: $id) {
      id identifier title description url priority
      state { id name type }
      assignee { id name }
    }
  }
`;

const UPDATE_ISSUE_STATE = gql`
  mutation UpdateIssueState($id: String!, $stateId: String!) {
    issueUpdate(id: $id, input: { stateId: $stateId }) {
      success
    }
  }
`;

const CREATE_ISSUE = gql`
  mutation CreateIssue($teamId: String!, $title: String!, $description: String) {
    issueCreate(input: { teamId: $teamId, title: $title, description: $description }) {
      success
      issue {
        id identifier title url priority
        state { id name type }
      }
    }
  }
`;

const GET_WORKFLOW_STATES = gql`
  query GetWorkflowStates($teamId: String!) {
    team(id: $teamId) {
      states { nodes { id name type position } }
    }
  }
`;

// ============ Helpers ============

function mapIssue(issue: IssueNode): LinearTicket {
  return {
    id: issue.id,
    identifier: issue.identifier,
    title: issue.title,
    description: issue.description ?? undefined,
    url: issue.url,
    priority: issue.priority,
    state: issue.state?.name ?? "Unknown",
    assignee: issue.assignee?.name,
  };
}

function getClient(): GraphQLClient {
  if (!client) throw new Error("Linear not initialized. Call initializeLinear() first.");
  return client;
}

// ============ API Functions ============

export async function searchTickets(query: string, limit = 20): Promise<LinearTicket[]> {
  const { issues } = await getClient().request<{ issues: { nodes: IssueNode[] } }>(
    SEARCH_ISSUES, { query, first: limit }
  );
  return issues.nodes.map(mapIssue);
}

export async function getTicket(identifier: string): Promise<LinearTicket | null> {
  try {
    const { issue } = await getClient().request<{ issue: IssueNode | null }>(
      GET_ISSUE, { id: identifier }
    );
    return issue ? mapIssue(issue) : null;
  } catch {
    return null;
  }
}

export async function updateTicketState(identifier: string, stateId: string): Promise<boolean> {
  const { issueUpdate } = await getClient().request<{ issueUpdate: { success: boolean } }>(
    UPDATE_ISSUE_STATE, { id: identifier, stateId }
  );
  return issueUpdate.success;
}

export async function createTicket(
  teamId: string,
  title: string,
  description?: string
): Promise<LinearTicket> {
  const { issueCreate } = await getClient().request<{ 
    issueCreate: { success: boolean; issue: IssueNode } 
  }>(CREATE_ISSUE, { teamId, title, description });
  return mapIssue(issueCreate.issue);
}

export async function getWorkflowStates(teamId: string): Promise<WorkflowState[]> {
  const { team } = await getClient().request<{ 
    team: { states: { nodes: WorkflowState[] } } 
  }>(GET_WORKFLOW_STATES, { teamId });
  return team.states.nodes.sort((a, b) => a.position - b.position);
}
```

This is the complete implementation - ~120 lines vs 37.7 MB SDK.

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| Linear REST API | GraphQL API | 2020 | All new features GraphQL-only |
| graphql-request direct | @linear/sdk | Current | Strongly typed, better DX |
| ink-text-input | @inkjs/ui TextInput | 2024 | More features, same author |
| ink-select-input | @inkjs/ui Select | 2024 | Better scroll, highlight |

**Deprecated/outdated:**
- `ink-text-input` v5: Still works but `@inkjs/ui` is the recommended replacement
- Linear REST API: Deprecated, use GraphQL
- `ink-select-input`: Works but `@inkjs/ui` Select has more features

## Workflow State Mappings

Linear workflow states have a `type` property indicating the category:

| State Type | Meaning | Example States |
|------------|---------|----------------|
| `backlog` | Not started, in backlog | Backlog, Todo |
| `unstarted` | Not yet started | Todo, Unstarted |
| `started` | In progress | In Progress, In Development |
| `completed` | Finished | Done, Completed |
| `canceled` | Cancelled/won't do | Cancelled, Won't Fix |

**Recommended status mapping for Agenator:**
| Session Event | Linear Status Type |
|---------------|-------------------|
| Session start | `started` (In Progress) |
| PR created | `started` (In Review) - if exists |
| PR merged | `completed` (Done) |
| Session killed | Keep current status |

## API Key Storage

**Recommended approach:**
1. **Primary:** Config file at `~/.config/agenator/config.yaml`
2. **Override:** Environment variable `AGENATOR_LINEAR_API_KEY`

```yaml
# ~/.config/agenator/config.yaml
linear:
  apiKey: lin_api_xxxxxxxxxxxxxxxxxxxx
  defaultTeam: ENG  # Optional, for filtering
```

**Security notes:**
- Config file should be `chmod 600` (owner read/write only)
- API key provides full access to user's Linear data
- Consider warning user if config file has wrong permissions

## Open Questions

Things that couldn't be fully resolved:

1. **Webhook vs Polling for PR merged detection**
   - What we know: Linear supports webhooks for issue updates
   - What's unclear: Whether PR merge triggers an issue update webhook, or if we need GitHub webhook
   - Recommendation: For v0.2.0, rely on manual refresh. Plan webhooks for v0.3.0

2. **Team-scoped search**
   - What we know: Config has `defaultTeam` field
   - What's unclear: Should search be restricted to default team, or allow cross-team?
   - Recommendation: Default to team-scoped, add `--all-teams` option later

3. **Bun compatibility**
   - What we know: Bun has 99% Node.js compatibility as of v1.2, @linear/sdk uses standard GraphQL
   - What's unclear: No explicit testing reports for @linear/sdk on Bun
   - Recommendation: Proceed with Bun, test early, fall back to Node if issues arise

## Web Search Findings (Jan 2026)

### Linear SDK Authentication (Verified)
From https://linear.app/developers/sdk:
- Personal API Keys: `new LinearClient({ apiKey: 'YOUR_PERSONAL_API_KEY' })`
- OAuth2: `new LinearClient({ accessToken: 'YOUR_OAUTH_ACCESS_TOKEN' })`
- **Recommendation:** Personal API key for CLI tool (simpler, no OAuth flow needed)

### Linear Search via GraphQL Filters (Verified)
From https://linear.app/developers/filtering:
- Use `containsIgnoreCase` for case-insensitive title search
- Combine conditions with `or` operator
- Example:
```graphql
query SearchIssuesByTitle {
  issues(filter: { title: { containsIgnoreCase: "keyword" } }) {
    nodes { id, title }
  }
}
```

### Updating Issue Status (Verified)
From https://linear.app/docs/configuring-workflows:
1. First fetch workflow states for the team to get state IDs
2. Update issue with `issueUpdate` mutation and `stateId`
```graphql
mutation {
  issueUpdate(id: "issue-id", input: { stateId: "in-review-state-id" }) {
    success
    issue { id, title, state { name } }
  }
}
```

### @inkjs/ui Components (Verified)
From https://www.npmjs.com/package/@inkjs/ui:
- `TextInput` with placeholder and onSubmit
- `Select` with options array and onChange
- Both support keyboard navigation out of the box

### Bun + @linear/sdk Compatibility (Caution)
From web search:
- Bun v0.7.0 fixed GraphQL module resolution issues
- Current Bun 1.3.6 should work, but test early
- Fallback: Can run with Node if issues arise

## Sources

### Primary (HIGH confidence)
- https://linear.app/developers/sdk - Official SDK getting started (WebSearch verified Jan 2026)
- https://linear.app/developers/filtering - GraphQL filter documentation (WebSearch verified Jan 2026)
- https://linear.app/docs/configuring-workflows - Workflow states documentation
- https://www.npmjs.com/package/@inkjs/ui - Ink UI components (WebSearch verified Jan 2026)
- Context7 /linear/linear - SDK methods, error handling, pagination
- Context7 /vadimdemedes/ink - useFocus, useFocusManager, useInput hooks

### Secondary (MEDIUM confidence)
- WebSearch verified: Linear search uses `filter.containsIgnoreCase` for text matching
- WebSearch verified: Status updates require fetching state IDs first
- WebSearch verified: @inkjs/ui provides TextInput and Select components

### Tertiary (LOW confidence)
- WebSearch only: Bun + @linear/sdk compatibility (Bun fixed GraphQL issues in v0.7.0, but explicit testing needed)

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH - graphql-request is mature and stable, @inkjs/ui well-documented
- Architecture: HIGH - Follows existing codebase patterns (services/, hooks/)
- API operations: HIGH - Verified with official Linear GraphQL docs
- Direct GraphQL vs SDK: HIGH - Clear winner on bundle size (60KB vs 37.7MB)
- Caching: MEDIUM - Pattern is sound but TTL may need tuning
- Bun compatibility: HIGH - graphql-request has minimal deps, lower risk than SDK

**Key decision:** Direct GraphQL API with graphql-request instead of @linear/sdk
- Rationale: 600x smaller bundle size, we only need 5 operations
- Risk: Manual type definitions (mitigated by providing complete implementation)

**Research date:** 2026-01-21
**Valid until:** 2026-02-21 (30 days - stable domain)
