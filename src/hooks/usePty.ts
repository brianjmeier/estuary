import { useState, useEffect, useRef, useCallback } from "react";
import { spawnPty, type PtyHandle } from "../services/pty.js";
import type { SessionStatus } from "../types.js";

const MAX_OUTPUT_LINES = 1000;

export interface UsePtyOptions {
  /** Command to run (e.g., "bash", "opencode") */
  command: string;
  /** Command arguments */
  args?: string[];
  /** Working directory for the PTY */
  cwd: string;
  /** Additional environment variables */
  env?: Record<string, string>;
  /** Terminal columns */
  cols?: number;
  /** Terminal rows */
  rows?: number;
  /** Callback when agent status changes (for status detection) */
  onStatusChange?: (status: SessionStatus) => void;
  /** Whether the PTY should be active (spawn on true, kill on false) */
  active?: boolean;
}

export interface UsePtyReturn {
  /** Output lines from the PTY */
  output: string[];
  /** Raw output buffer (preserves ANSI codes) */
  rawOutput: string;
  /** Write input to the PTY */
  write: (input: string) => void;
  /** Kill the PTY process */
  kill: () => void;
  /** Resize the PTY terminal */
  resize: (cols: number, rows: number) => void;
  /** Whether the PTY process is currently running */
  isRunning: boolean;
  /** Exit code if process has exited */
  exitCode: number | null;
  /** Clear the output buffer */
  clearOutput: () => void;
}

/**
 * React hook to manage a PTY process lifecycle
 */
export function usePty(opts: UsePtyOptions): UsePtyReturn {
  const [output, setOutput] = useState<string[]>([]);
  const [rawOutput, setRawOutput] = useState("");
  const [isRunning, setIsRunning] = useState(false);
  const [exitCode, setExitCode] = useState<number | null>(null);
  
  const ptyRef = useRef<PtyHandle | null>(null);
  const rawBufferRef = useRef("");

  // Parse raw output into lines
  const appendOutput = useCallback((data: string) => {
    rawBufferRef.current += data;
    setRawOutput(rawBufferRef.current);
    
    // Split into lines, keeping partial lines
    setOutput((prev) => {
      // Combine last partial line with new data if needed
      const combined = prev.length > 0 
        ? prev.slice(0, -1).concat(prev[prev.length - 1] + data)
        : [data];
      
      // Re-split on newlines
      const allText = combined.join("");
      const lines = allText.split(/\r?\n/);
      
      // Keep only the last MAX_OUTPUT_LINES
      if (lines.length > MAX_OUTPUT_LINES) {
        return lines.slice(-MAX_OUTPUT_LINES);
      }
      return lines;
    });
  }, []);

  // Spawn PTY
  useEffect(() => {
    const active = opts.active ?? true;
    
    if (!active) {
      // If not active, don't spawn
      return;
    }

    // Reset state for new spawn
    setOutput([]);
    setRawOutput("");
    setExitCode(null);
    rawBufferRef.current = "";

    const pty = spawnPty({
      command: opts.command,
      args: opts.args,
      cwd: opts.cwd,
      env: opts.env,
      cols: opts.cols,
      rows: opts.rows,
      onData: (data) => {
        appendOutput(data);
        // TODO: Status detection from output patterns
        // e.g., detect "waiting for input" patterns
      },
      onExit: (code) => {
        setIsRunning(false);
        setExitCode(code);
        ptyRef.current = null;
        
        // Update status based on exit code
        if (opts.onStatusChange) {
          opts.onStatusChange(code === 0 ? "done" : "error");
        }
      },
    });

    ptyRef.current = pty;
    setIsRunning(true);
    
    if (opts.onStatusChange) {
      opts.onStatusChange("working");
    }

    // Cleanup on unmount or when dependencies change
    return () => {
      if (ptyRef.current) {
        ptyRef.current.kill();
        ptyRef.current = null;
      }
    };
  }, [opts.command, opts.cwd, opts.active, appendOutput]);

  const write = useCallback((input: string) => {
    ptyRef.current?.write(input);
  }, []);

  const kill = useCallback(() => {
    ptyRef.current?.kill();
    ptyRef.current = null;
    setIsRunning(false);
  }, []);

  const resize = useCallback((cols: number, rows: number) => {
    ptyRef.current?.resize(cols, rows);
  }, []);

  const clearOutput = useCallback(() => {
    setOutput([]);
    setRawOutput("");
    rawBufferRef.current = "";
  }, []);

  return {
    output,
    rawOutput,
    write,
    kill,
    resize,
    isRunning,
    exitCode,
    clearOutput,
  };
}
