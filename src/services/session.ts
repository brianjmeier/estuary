import { spawnPty, killPty, type PtyHandle } from "./pty.js";
import type { Session, SessionStatus, LinearTicket } from "../types.js";

export interface SessionConfig {
  /** Agent command to run */
  agentCommand: string;
  /** Agent command arguments */
  agentArgs?: string[];
  /** Base directory for workspaces */
  workspaceBaseDir: string;
}

export interface ManagedSession extends Omit<Session, 'outputBuffer'> {
  pty?: PtyHandle;
  /** Output buffer as array of chunks for easier processing */
  outputChunks: string[];
  /** Full output buffer as string (for Session compatibility) */
  outputBuffer: string;
}

type SessionUpdateCallback = (session: ManagedSession) => void;
type OutputCallback = (sessionId: string, data: string) => void;

// Session registry
const sessions = new Map<string, ManagedSession>();
const sessionUpdateCallbacks = new Set<SessionUpdateCallback>();
const outputCallbacks = new Set<OutputCallback>();

let nextSessionId = 1;

/**
 * Create a new session with a PTY
 */
export function createSession(opts: {
  name: string;
  workspacePath: string;
  sourceRepo: string;
  agentCommand: string;
  agentArgs?: string[];
  ticketId?: string;
  ticketTitle?: string;
}): ManagedSession {
  const id = `session-${nextSessionId++}`;
  
  const session: ManagedSession = {
    id,
    name: opts.name,
    status: "idle",
    workspacePath: opts.workspacePath,
    sourceRepo: opts.sourceRepo,
    ticketId: opts.ticketId,
    ticketTitle: opts.ticketTitle,
    lastOutput: [],
    outputChunks: [],
    outputBuffer: "",
    createdAt: new Date(),
    updatedAt: new Date(),
  };

  // Spawn PTY for the session
  const pty = spawnPty({
    command: opts.agentCommand,
    args: opts.agentArgs,
    cwd: opts.workspacePath,
    onData: (data) => {
      // Append to output chunks
      session.outputChunks.push(data);
      session.outputBuffer += data;
      
      // Update lastOutput with recent lines
      const allText = session.outputChunks.join("");
      const lines = allText.split(/\r?\n/);
      session.lastOutput = lines.slice(-100); // Keep last 100 lines
      
      // Notify listeners
      outputCallbacks.forEach((cb) => cb(session.id, data));
      notifySessionUpdate(session);
    },
    onExit: (code) => {
      session.status = code === 0 ? "done" : "error";
      session.pty = undefined;
      session.updatedAt = new Date();
      notifySessionUpdate(session);
    },
  });

  session.pty = pty;
  session.status = "working";
  
  sessions.set(id, session);
  notifySessionUpdate(session);
  
  return session;
}

/**
 * Get a session by ID
 */
export function getSession(id: string): ManagedSession | undefined {
  return sessions.get(id);
}

/**
 * Get all sessions
 */
export function getAllSessions(): ManagedSession[] {
  return Array.from(sessions.values());
}

/**
 * Update session status
 */
export function updateSessionStatus(id: string, status: SessionStatus): void {
  const session = sessions.get(id);
  if (session) {
    session.status = status;
    session.updatedAt = new Date();
    notifySessionUpdate(session);
  }
}

/**
 * Write input to a session's PTY
 */
export function writeToSession(id: string, input: string): void {
  const session = sessions.get(id);
  session?.pty?.write(input);
}

/**
 * Kill a session's PTY
 */
export function killSession(id: string): void {
  const session = sessions.get(id);
  if (session?.pty) {
    killPty(session.pty);
    session.pty = undefined;
    session.status = "idle";
    session.updatedAt = new Date();
    notifySessionUpdate(session);
  }
}

/**
 * Remove a session completely
 */
export function removeSession(id: string): void {
  const session = sessions.get(id);
  if (session) {
    if (session.pty) {
      killPty(session.pty);
    }
    sessions.delete(id);
  }
}

/**
 * Kill all sessions' PTYs
 */
export function killAllSessions(): void {
  for (const session of sessions.values()) {
    if (session.pty) {
      killPty(session.pty);
      session.pty = undefined;
    }
  }
}

/**
 * Subscribe to session updates
 */
export function onSessionUpdate(callback: SessionUpdateCallback): () => void {
  sessionUpdateCallbacks.add(callback);
  return () => sessionUpdateCallbacks.delete(callback);
}

/**
 * Subscribe to session output
 */
export function onSessionOutput(callback: OutputCallback): () => void {
  outputCallbacks.add(callback);
  return () => outputCallbacks.delete(callback);
}

function notifySessionUpdate(session: ManagedSession): void {
  sessionUpdateCallbacks.forEach((cb) => cb(session));
}

/**
 * Generate workspace name from ticket
 * Uses ticket identifier (e.g., "ENG-123")
 */
export function getWorkspaceNameFromTicket(ticket: LinearTicket): string {
  return ticket.identifier;
}

/**
 * Update session with linked ticket
 */
export function linkTicketToSession(id: string, ticket: LinearTicket): void {
  const session = sessions.get(id);
  if (session) {
    session.ticketId = ticket.identifier;
    session.ticketTitle = ticket.title;
    session.name = ticket.identifier; // Auto-name from ticket
    session.updatedAt = new Date();
    notifySessionUpdate(session);
  }
}
