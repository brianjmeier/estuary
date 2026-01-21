import { Box, Text } from "ink";
import type { SessionStatus } from "../types.js";

interface StatusBadgeProps {
  status: SessionStatus;
}

export function StatusBadge({ status }: StatusBadgeProps) {
  const config = getStatusConfig(status);
  
  return (
    <Box>
      <Text backgroundColor={config.bg} color={config.fg} bold>
        {" "}{config.label}{" "}
      </Text>
    </Box>
  );
}

interface StatusConfig {
  label: string;
  bg: string;
  fg: string;
}

function getStatusConfig(status: SessionStatus): StatusConfig {
  switch (status) {
    case "needs_input":
      return { label: "NEEDS INPUT", bg: "yellow", fg: "black" };
    case "working":
      return { label: "WORKING", bg: "blue", fg: "white" };
    case "error":
      return { label: "ERROR", bg: "red", fg: "white" };
    case "paused":
      return { label: "PAUSED", bg: "gray", fg: "white" };
    case "done":
      return { label: "DONE", bg: "green", fg: "black" };
    case "idle":
    default:
      return { label: "READY", bg: "cyan", fg: "black" };
  }
}
