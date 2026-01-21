// Session status - what the agent is currently doing
export type SessionStatus = 
  | "idle"        // Not started or completed
  | "working"     // Agent is processing/thinking
  | "needs_input" // Agent is waiting for user input
  | "error"       // Something went wrong
  | "paused"      // Manually paused
  | "done";       // Marked as done, ready for PR

// A single agent session
export interface Session {
  id: string;
  name: string;
  status: SessionStatus;
  
  // Linear integration
  ticketId?: string;        // e.g., "ENG-123"
  ticketTitle?: string;
  ticketUrl?: string;
  
  // JJ Workspace
  workspacePath: string;    // e.g., ~/.agenator/workspaces/ENG-123
  sourceRepo: string;       // The repo this workspace was created from
  revisionId?: string;      // JJ revision ID for this work
  
  // Git context (after push)
  branch?: string;          // e.g., agenator/ENG-123-user-auth
  
  // PR info
  prNumber?: number;
  prUrl?: string;
  prStatus?: "draft" | "open" | "merged" | "closed";
  
  // Terminal state
  lastOutput: string[];     // Recent terminal output lines
  outputBuffer: string;     // Full output buffer
  
  // Timestamps
  createdAt: Date;
  updatedAt: Date;
}

// JJ Workspace info
export interface Workspace {
  path: string;
  name: string;             // Workspace name in JJ
  sourceRepo: string;
  revisionId: string;
  isActive: boolean;
}

// Linear ticket info
export interface LinearTicket {
  id: string;
  identifier: string;      // e.g., "ENG-123"
  title: string;
  description?: string;
  url: string;
  state: string;
  priority: number;
  assignee?: string;
}

// GitHub PR info
export interface PullRequest {
  number: number;
  title: string;
  url: string;
  state: "open" | "closed" | "merged";
  draft: boolean;
  reviewComments: PRComment[];
}

export interface PRComment {
  id: number;
  body: string;
  path?: string;
  line?: number;
  author: string;
  createdAt: Date;
}

// App configuration
export interface Config {
  linear: {
    apiKey?: string;
    defaultTeam?: string;
  };
  agent: {
    command: string;        // "opencode" | "claude" | custom
    args?: string[];
  };
  workspace: {
    baseDir: string;        // ~/.agenator/workspaces
    sourceRepo: string;     // Default repo to create workspaces from
    branchPrefix: string;   // e.g., "agenator/"
    cleanupOnMerge: boolean;
  };
  ui: {
    theme: "dark" | "light";
    notifyOnInput: boolean;
  };
}

// Actions available in the UI
export type Action =
  | { type: "create_session"; name?: string; ticketId?: string }
  | { type: "close_session"; sessionId: string }
  | { type: "switch_session"; sessionId: string }
  | { type: "link_ticket"; sessionId: string; ticketId: string }
  | { type: "create_ticket"; sessionId: string; title: string; description?: string }
  | { type: "mark_done"; sessionId: string }
  | { type: "create_pr"; sessionId: string }
  | { type: "cleanup_workspace"; sessionId: string }
  | { type: "send_input"; sessionId: string; input: string }
  | { type: "pause_session"; sessionId: string }
  | { type: "resume_session"; sessionId: string };
