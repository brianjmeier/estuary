import { Box, Text } from "ink";
import type { Session } from "../types.js";

interface ActionBarProps {
  onAction: (action: string) => void;
  hasNeedsInput: boolean;
  activeSession?: Session;
  inputMode?: boolean;
}

export function ActionBar({ hasNeedsInput, activeSession, inputMode }: ActionBarProps) {
  const isDone = activeSession?.status === "done";
  const hasPR = !!activeSession?.prNumber;

  return (
    <Box borderStyle="single" borderColor="gray" paddingX={1} justifyContent="space-between">
      <Box>
        {inputMode ? (
          // Input mode: show minimal actions
          <Text color="gray">
            <Text color="yellow" bold>[Esc]</Text> exit input mode
          </Text>
        ) : (
          // Command mode: show all actions
          <>
            <ActionKey keyName="i" label="Input" highlight />
            <ActionKey keyName="n" label="New" />
            <ActionKey keyName="d" label="Done" disabled={isDone} />
            <ActionKey keyName="p" label="Create PR" disabled={!isDone || hasPR} />
            <ActionKey keyName="l" label="Link Ticket" />
            <ActionKey keyName="x" label="Kill" />
            <ActionKey keyName="?" label="Help" />
          </>
        )}
      </Box>
      <Box>
        {inputMode && (
          <Text backgroundColor="blue" color="white" bold>
            {" "}INPUT MODE{" "}
          </Text>
        )}
        {!inputMode && hasNeedsInput && (
          <Text backgroundColor="yellow" color="black" bold>
            {" "}AGENT NEEDS INPUT{" "}
          </Text>
        )}
        {!inputMode && isDone && !hasPR && (
          <Text backgroundColor="green" color="black" bold>
            {" "}READY FOR PR{" "}
          </Text>
        )}
        {!inputMode && hasPR && activeSession?.prStatus === "open" && (
          <Text backgroundColor="magenta" color="white" bold>
            {" "}PR #{activeSession.prNumber}{" "}
          </Text>
        )}
        {!inputMode && <Text color="gray"> Ctrl+C quit</Text>}
      </Box>
    </Box>
  );
}

interface ActionKeyProps {
  keyName: string;
  label: string;
  disabled?: boolean;
  highlight?: boolean;
}

function ActionKey({ keyName, label, disabled, highlight }: ActionKeyProps) {
  const keyColor = disabled ? "gray" : highlight ? "yellow" : "cyan";
  
  return (
    <Box marginRight={2}>
      <Text color={keyColor} bold dimColor={disabled}>
        [{keyName}]
      </Text>
      <Text color="gray" dimColor={disabled}> {label}</Text>
    </Box>
  );
}
