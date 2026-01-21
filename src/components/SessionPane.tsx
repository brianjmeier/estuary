import { Box, Text } from "ink";
import type { Session } from "../types.js";
import { StatusBadge } from "./StatusBadge.js";

interface SessionPaneProps {
  session: Session;
  /** Live PTY output (overrides session.lastOutput if provided) */
  ptyOutput?: string[];
  /** Whether PTY is currently running */
  ptyRunning?: boolean;
  /** Whether in input mode */
  inputMode?: boolean;
}

export function SessionPane({ 
  session, 
  ptyOutput, 
  ptyRunning,
  inputMode 
}: SessionPaneProps) {
  // Use PTY output if available, otherwise fall back to session.lastOutput
  const outputLines = ptyOutput ?? session.lastOutput;
  
  // Show loading state if PTY should be running but no output yet
  const isLoading = ptyRunning && outputLines.length === 0;

  return (
    <Box flexDirection="column" flexGrow={1} borderStyle="round" borderColor="cyan">
      {/* Header */}
      <Box paddingX={1} borderStyle="single" borderBottom borderColor="gray">
        <Box flexGrow={1}>
          {session.ticketId && (
            <Text bold color="yellow">{session.ticketId}</Text>
          )}
          {session.ticketTitle && (
            <Text color="white"> {session.ticketTitle}</Text>
          )}
          {!session.ticketId && !session.ticketTitle && (
            <Text color="gray">No ticket linked</Text>
          )}
        </Box>
        <Box>
          {inputMode && (
            <Text backgroundColor="blue" color="white" bold>
              {" "}INPUT{" "}
            </Text>
          )}
          <Text> </Text>
          <StatusBadge status={session.status} />
        </Box>
      </Box>

      {/* Workspace & Branch info */}
      <Box paddingX={1} paddingY={0} flexDirection="column">
        <Box>
          <Text color="gray">workspace: </Text>
          <Text color="blue">{session.workspacePath}</Text>
        </Box>
        {session.branch && (
          <Box>
            <Text color="gray">branch: </Text>
            <Text color="green">{session.branch}</Text>
            {session.prNumber && (
              <>
                <Text color="gray"> | PR: </Text>
                <Text color="magenta">#{session.prNumber}</Text>
                {session.prStatus && (
                  <Text color="gray"> ({session.prStatus})</Text>
                )}
              </>
            )}
          </Box>
        )}
      </Box>

      {/* Terminal output */}
      <Box flexDirection="column" flexGrow={1} paddingX={1} paddingY={1} overflow="hidden">
        {isLoading ? (
          <Text color="gray">Starting agent...</Text>
        ) : (
          // Show last ~20 visible lines (auto-scroll effect)
          outputLines.slice(-20).map((line, i) => (
            <Text key={i} wrap="truncate">
              {line || " "}
            </Text>
          ))
        )}
      </Box>
    </Box>
  );
}
