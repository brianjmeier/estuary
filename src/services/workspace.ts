/**
 * JJ Workspace Management
 * 
 * Handles creating, managing, and cleaning up Jujutsu workspaces
 * for agent sessions.
 */

import { $ } from "bun";
import * as fs from "fs/promises";
import * as path from "path";

export interface CreateWorkspaceOptions {
  sessionId: string;
  ticketId?: string;
  sourceRepo: string;
  baseBranch?: string;
}

export interface WorkspaceInfo {
  path: string;
  name: string;
  revisionId: string;
  sourceRepo: string;
}

const DEFAULT_BASE_DIR = `${process.env.HOME}/.agenator/workspaces`;
const BRANCH_PREFIX = "agenator/";

/**
 * Create a new JJ workspace for an agent session
 */
export async function createWorkspace(opts: CreateWorkspaceOptions): Promise<WorkspaceInfo> {
  const workspaceName = opts.ticketId || opts.sessionId;
  const workspacePath = path.join(DEFAULT_BASE_DIR, workspaceName);
  const baseBranch = opts.baseBranch || "main";

  // Ensure base directory exists
  await fs.mkdir(DEFAULT_BASE_DIR, { recursive: true });

  // Check if workspace already exists
  try {
    await fs.access(workspacePath);
    throw new Error(`Workspace already exists: ${workspacePath}`);
  } catch (err: unknown) {
    if ((err as NodeJS.ErrnoException).code !== "ENOENT") throw err;
  }

  // Create the JJ workspace
  await $`jj workspace add ${workspacePath} --revision ${baseBranch}`
    .cwd(opts.sourceRepo)
    .quiet();

  // Create a new revision for this agent's work
  const description = opts.ticketId 
    ? `agenator: ${opts.ticketId} - Work in progress`
    : `agenator: ${opts.sessionId} - Work in progress`;
  
  await $`jj new -m ${description}`.cwd(workspacePath).quiet();

  // Get the revision ID
  const revisionId = await $`jj log -r @ --no-graph -T 'change_id'`
    .cwd(workspacePath)
    .text();

  return {
    path: workspacePath,
    name: workspaceName,
    revisionId: revisionId.trim(),
    sourceRepo: opts.sourceRepo,
  };
}

/**
 * Clean up a workspace after PR is merged or session is killed
 */
export async function cleanupWorkspace(workspacePath: string): Promise<void> {
  const workspaceName = path.basename(workspacePath);

  try {
    // Get the source repo from the workspace
    const sourceRepo = await $`jj workspace root`.cwd(workspacePath).text();
    
    // Forget the workspace from JJ
    await $`jj workspace forget ${workspaceName}`
      .cwd(sourceRepo.trim())
      .quiet();
  } catch {
    // Workspace might already be forgotten, continue with directory cleanup
  }

  // Remove the directory
  await fs.rm(workspacePath, { recursive: true, force: true });
}

/**
 * Push the workspace changes to a git branch
 */
export async function pushWorkspace(
  workspacePath: string, 
  branchName: string,
  commitMessage?: string
): Promise<void> {
  // Update the revision description if provided
  if (commitMessage) {
    await $`jj describe -m ${commitMessage}`.cwd(workspacePath).quiet();
  }

  // Create a bookmark (JJ's equivalent of a branch)
  try {
    await $`jj bookmark create ${branchName} -r @`.cwd(workspacePath).quiet();
  } catch {
    // Bookmark might already exist, try to move it
    await $`jj bookmark move ${branchName} --to @`.cwd(workspacePath).quiet();
  }

  // Push to remote
  await $`jj git push --bookmark ${branchName}`.cwd(workspacePath);
}

/**
 * Get the diff of changes in the workspace
 */
export async function getWorkspaceDiff(workspacePath: string): Promise<string> {
  const diff = await $`jj diff`.cwd(workspacePath).text();
  return diff;
}

/**
 * Get the status of the workspace
 */
export async function getWorkspaceStatus(workspacePath: string): Promise<string> {
  const status = await $`jj status`.cwd(workspacePath).text();
  return status;
}

/**
 * List all agenator workspaces
 */
export async function listWorkspaces(): Promise<WorkspaceInfo[]> {
  const workspaces: WorkspaceInfo[] = [];
  
  try {
    const entries = await fs.readdir(DEFAULT_BASE_DIR, { withFileTypes: true });
    
    for (const entry of entries) {
      if (entry.isDirectory()) {
        const workspacePath = path.join(DEFAULT_BASE_DIR, entry.name);
        try {
          const revisionId = await $`jj log -r @ --no-graph -T 'change_id'`
            .cwd(workspacePath)
            .text();
          const sourceRepo = await $`jj workspace root`
            .cwd(workspacePath)
            .text();
          
          workspaces.push({
            path: workspacePath,
            name: entry.name,
            revisionId: revisionId.trim(),
            sourceRepo: sourceRepo.trim(),
          });
        } catch {
          // Not a valid JJ workspace, skip
        }
      }
    }
  } catch {
    // Base directory doesn't exist yet
  }
  
  return workspaces;
}

/**
 * Check if JJ is installed
 */
export async function checkJJInstalled(): Promise<boolean> {
  try {
    await $`jj --version`.quiet();
    return true;
  } catch {
    return false;
  }
}

/**
 * Generate branch name from ticket
 * Format: agenator/ENG-123-<slug>
 * Slug from title: lowercase, hyphenated, max 40 chars
 */
export function generateBranchName(ticketId: string, ticketTitle: string): string {
  // Create slug from title
  const slug = ticketTitle
    .toLowerCase()
    .replace(/[^a-z0-9\s-]/g, "") // Remove special chars except hyphens
    .replace(/\s+/g, "-")          // Replace spaces with hyphens
    .replace(/-+/g, "-")           // Collapse multiple hyphens
    .slice(0, 40)                  // Max 40 chars
    .replace(/-$/, "");            // Remove trailing hyphen
  
  return `${BRANCH_PREFIX}${ticketId}-${slug}`;
}

/**
 * Generate branch name from ticket object
 */
export function getBranchNameFromTicket(ticket: { identifier: string; title: string }): string {
  return generateBranchName(ticket.identifier, ticket.title);
}
