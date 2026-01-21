/**
 * Linear GraphQL API Client
 * 
 * Lightweight wrapper around graphql-request for Linear operations.
 * ~120 lines vs 37.7 MB SDK - we only need 5 operations.
 */

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

export interface WorkflowState {
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

/**
 * Find the "In Progress" state for a team
 * Returns the first state with type "started"
 */
export async function findInProgressState(teamId: string): Promise<WorkflowState | null> {
  const states = await getWorkflowStates(teamId);
  return states.find(s => s.type === "started") ?? null;
}

/**
 * Move ticket to "In Progress" status
 * Returns true if successful, false if no started state found
 */
export async function moveTicketToInProgress(
  ticketIdentifier: string, 
  teamId: string
): Promise<boolean> {
  const inProgressState = await findInProgressState(teamId);
  if (!inProgressState) return false;
  
  return updateTicketState(ticketIdentifier, inProgressState.id);
}
