import { Box, Text } from "ink";
import type { Session } from "../types.js";

interface TabBarProps {
  sessions: Session[];
  activeSessionId: string;
  onSelect: (id: string) => void;
}

export function TabBar({ sessions, activeSessionId }: TabBarProps) {
  return (
    <Box borderStyle="single" borderColor="gray" paddingX={1}>
      {sessions.map((session, index) => (
        <Tab
          key={session.id}
          session={session}
          index={index + 1}
          isActive={session.id === activeSessionId}
        />
      ))}
      <Box marginLeft={1}>
        <Text color="gray">[+] new</Text>
      </Box>
    </Box>
  );
}

interface TabProps {
  session: Session;
  index: number;
  isActive: boolean;
}

function Tab({ session, index, isActive }: TabProps) {
  const statusIndicator = getStatusIndicator(session.status);
  const bgColor = isActive ? "blue" : undefined;
  const textColor = isActive ? "white" : "gray";

  return (
    <Box marginRight={1}>
      <Text backgroundColor={bgColor} color={textColor}>
        {" "}
        <Text bold>{index}</Text>
        {" "}
        {statusIndicator}
        {" "}
        {session.ticketId ? `${session.ticketId}: ` : ""}
        {session.name}
        {" "}
      </Text>
    </Box>
  );
}

function getStatusIndicator(status: Session["status"]): string {
  switch (status) {
    case "needs_input":
      return "!";  // Attention needed
    case "working":
      return "*";  // In progress
    case "error":
      return "x";  // Error
    case "paused":
      return "-";  // Paused
    case "done":
      return "+";  // Done
    case "idle":
    default:
      return " ";  // Ready/idle
  }
}
