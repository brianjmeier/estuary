import { useState, useEffect, useCallback } from "react";
import { Box, useInput, useApp } from "ink";
import { TabBar } from "./components/TabBar.js";
import { SessionPane } from "./components/SessionPane.js";
import { ActionBar } from "./components/ActionBar.js";
import { usePty } from "./hooks/usePty.js";
import { killAllPtys } from "./services/pty.js";
import type { Session, SessionStatus } from "./types.js";

// For development: use bash as the agent command
const DEV_AGENT_COMMAND = "bash";
const DEV_AGENT_CWD = process.cwd();

// Mock sessions for UI development (will be replaced with real session management)
const createMockSession = (id: string, name: string): Session => ({
  id,
  name,
  status: "idle",
  workspacePath: DEV_AGENT_CWD,
  sourceRepo: DEV_AGENT_CWD,
  lastOutput: [],
  outputBuffer: "",
  createdAt: new Date(),
  updatedAt: new Date(),
});

export function App() {
  const { exit } = useApp();
  const [sessions, setSessions] = useState<Session[]>([
    createMockSession("1", "Session 1"),
  ]);
  const [activeSessionId, setActiveSessionId] = useState<string>("1");
  const [inputMode, setInputMode] = useState(false);

  const activeSession = sessions.find((s) => s.id === activeSessionId);
  const hasNeedsInput = sessions.some((s) => s.status === "needs_input");

  // Update session status
  const updateSessionStatus = useCallback((sessionId: string, status: SessionStatus) => {
    setSessions((prev) =>
      prev.map((s) => (s.id === sessionId ? { ...s, status, updatedAt: new Date() } : s))
    );
  }, []);

  // PTY for active session
  const {
    output: ptyOutput,
    write: ptyWrite,
    isRunning: ptyRunning,
    kill: ptyKill,
  } = usePty({
    command: DEV_AGENT_COMMAND,
    cwd: activeSession?.workspacePath ?? DEV_AGENT_CWD,
    active: !!activeSession,
    onStatusChange: (status) => {
      if (activeSession) {
        updateSessionStatus(activeSession.id, status);
      }
    },
  });

  // Cleanup all PTYs on app exit
  useEffect(() => {
    return () => {
      killAllPtys();
    };
  }, []);

  useInput((input, key) => {
    // Always allow Ctrl+C to quit
    if (key.ctrl && input === "c") {
      killAllPtys();
      exit();
      return;
    }

    // Input mode: forward everything except Escape to PTY
    if (inputMode) {
      if (key.escape) {
        setInputMode(false);
        return;
      }
      
      // Forward key to PTY
      if (key.return) {
        ptyWrite("\n");
      } else if (key.backspace || key.delete) {
        ptyWrite("\x7f"); // DEL character
      } else if (key.upArrow) {
        ptyWrite("\x1b[A");
      } else if (key.downArrow) {
        ptyWrite("\x1b[B");
      } else if (key.leftArrow) {
        ptyWrite("\x1b[D");
      } else if (key.rightArrow) {
        ptyWrite("\x1b[C");
      } else if (key.tab) {
        ptyWrite("\t");
      } else if (key.ctrl && input) {
        // Send Ctrl+key as control character
        const code = input.charCodeAt(0) - 96; // 'a' -> 1, etc.
        if (code > 0 && code < 27) {
          ptyWrite(String.fromCharCode(code));
        }
      } else if (input) {
        ptyWrite(input);
      }
      return;
    }

    // Command mode: handle app navigation
    
    // Enter input mode
    if (input === "i") {
      setInputMode(true);
      return;
    }

    // Tab navigation with number keys
    if (input >= "1" && input <= "9") {
      const index = parseInt(input) - 1;
      if (sessions[index]) {
        setActiveSessionId(sessions[index].id);
      }
    }

    // Next/prev tab
    if (key.tab || (key.ctrl && input === "n")) {
      const currentIndex = sessions.findIndex((s) => s.id === activeSessionId);
      const nextIndex = (currentIndex + 1) % sessions.length;
      setActiveSessionId(sessions[nextIndex].id);
    }
    if (key.shift && key.tab) {
      const currentIndex = sessions.findIndex((s) => s.id === activeSessionId);
      const prevIndex = (currentIndex - 1 + sessions.length) % sessions.length;
      setActiveSessionId(sessions[prevIndex].id);
    }

    // Create new session
    if (input === "n") {
      const newId = String(sessions.length + 1);
      const newSession = createMockSession(newId, `Session ${newId}`);
      setSessions((prev) => [...prev, newSession]);
      setActiveSessionId(newId);
    }

    // Mark as done
    if (input === "d" && activeSession) {
      updateSessionStatus(activeSession.id, "done");
    }

    // Kill session
    if (input === "x" && activeSession) {
      ptyKill();
      updateSessionStatus(activeSession.id, "idle");
    }
  });

  const handleAction = (action: string) => {
    switch (action) {
      case "new":
        const newId = String(sessions.length + 1);
        const newSession = createMockSession(newId, `Session ${newId}`);
        setSessions((prev) => [...prev, newSession]);
        setActiveSessionId(newId);
        break;
      case "done":
        if (activeSession) {
          updateSessionStatus(activeSession.id, "done");
        }
        break;
      case "link":
        // TODO: Link Linear ticket
        break;
      case "pr":
        // TODO: Create PR from workspace
        break;
      case "cleanup":
        // TODO: Cleanup workspace
        break;
    }
  };

  return (
    <Box flexDirection="column" height="100%">
      <TabBar
        sessions={sessions}
        activeSessionId={activeSessionId}
        onSelect={setActiveSessionId}
      />
      {activeSession && (
        <SessionPane
          session={activeSession}
          ptyOutput={ptyOutput}
          ptyRunning={ptyRunning}
          inputMode={inputMode}
        />
      )}
      <ActionBar
        onAction={handleAction}
        hasNeedsInput={hasNeedsInput}
        activeSession={activeSession}
        inputMode={inputMode}
      />
    </Box>
  );
}
