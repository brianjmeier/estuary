import type { Subprocess } from "bun";

export interface PtyHandle {
  id: string;
  write(input: string): void;
  resize(cols: number, rows: number): void;
  kill(): void;
  readonly isRunning: boolean;
}

export interface SpawnOptions {
  command: string;
  args?: string[];
  cwd: string;
  env?: Record<string, string>;
  cols?: number;
  rows?: number;
  onData?: (data: string) => void;
  onExit?: (code: number) => void;
}

// Registry of all active PTYs for cleanup
const activePtys = new Map<string, PtyProcess>();

let nextId = 1;

class PtyProcess implements PtyHandle {
  id: string;
  private proc: Subprocess<"ignore", "ignore", "ignore"> | null = null;
  private _isRunning = false;
  private onExitCallback?: (code: number) => void;

  constructor(opts: SpawnOptions) {
    this.id = `pty-${nextId++}`;
    this.spawn(opts);
  }

  private spawn(opts: SpawnOptions) {
    const cmd = [opts.command, ...(opts.args || [])];
    
    this.proc = Bun.spawn(cmd, {
      cwd: opts.cwd,
      env: {
        ...process.env,
        ...opts.env,
        // Ensure proper terminal behavior
        TERM: "xterm-256color",
        COLORTERM: "truecolor",
      },
      terminal: {
        cols: opts.cols ?? 80,
        rows: opts.rows ?? 24,
        data: (_terminal, data: Uint8Array) => {
          // Convert Uint8Array to string
          const text = new TextDecoder().decode(data);
          opts.onData?.(text);
        },
      },
    });

    this._isRunning = true;
    this.onExitCallback = opts.onExit;

    // Handle process exit
    this.proc.exited.then((code) => {
      this._isRunning = false;
      activePtys.delete(this.id);
      this.onExitCallback?.(code ?? 0);
    }).catch((err) => {
      console.error(`PTY ${this.id} error:`, err);
      this._isRunning = false;
      activePtys.delete(this.id);
      this.onExitCallback?.(-1);
    });

    activePtys.set(this.id, this);
  }

  get isRunning(): boolean {
    return this._isRunning;
  }

  write(input: string): void {
    if (!this.proc || !this._isRunning) {
      console.warn(`PTY ${this.id}: Cannot write to non-running process`);
      return;
    }
    (this.proc as any).terminal?.write(input);
  }

  resize(cols: number, rows: number): void {
    if (!this.proc || !this._isRunning) {
      return;
    }
    (this.proc as any).terminal?.resize?.(cols, rows);
  }

  kill(): void {
    if (!this.proc) {
      return;
    }
    
    try {
      // Close terminal first
      (this.proc as any).terminal?.close();
      // Then kill the process
      this.proc.kill();
    } catch {
      // Process may already be dead
    }
    
    this._isRunning = false;
    activePtys.delete(this.id);
  }
}

/**
 * Spawn a new PTY process
 */
export function spawnPty(opts: SpawnOptions): PtyHandle {
  return new PtyProcess(opts);
}

/**
 * Kill a PTY by handle
 */
export function killPty(handle: PtyHandle): void {
  handle.kill();
}

/**
 * Get all active PTY handles
 */
export function getActivePtys(): PtyHandle[] {
  return Array.from(activePtys.values());
}

/**
 * Kill all active PTYs - call on app exit
 */
export function killAllPtys(): void {
  for (const pty of activePtys.values()) {
    pty.kill();
  }
  activePtys.clear();
}

/**
 * Get count of active PTYs
 */
export function getActivePtyCount(): number {
  return activePtys.size;
}
