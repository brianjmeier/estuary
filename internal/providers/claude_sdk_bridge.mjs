import { query } from "@anthropic-ai/claude-agent-sdk";
import readline from "node:readline";

function write(message) {
  process.stdout.write(JSON.stringify(message) + "\n");
}

function createPromptQueue() {
  const queued = [];
  const waiters = [];
  let closed = false;

  return {
    push(item) {
      if (closed) {
        throw new Error("Prompt queue is closed.");
      }
      const waiter = waiters.shift();
      if (waiter) {
        waiter(item);
        return;
      }
      queued.push(item);
    },
    close() {
      closed = true;
      while (waiters.length > 0) {
        const waiter = waiters.shift();
        waiter?.({ type: "terminate" });
      }
    },
    async *stream() {
      while (true) {
        if (queued.length > 0) {
          const item = queued.shift();
          if (item?.type === "terminate") {
            return;
          }
          yield item;
          continue;
        }
        const item = await new Promise((resolve) => {
          waiters.push(resolve);
        });
        if (item?.type === "terminate") {
          return;
        }
        yield item;
      }
    },
  };
}

let sessionConfig = null;
let promptQueue = null;
let activeQuery = null;
let sessionId = null;
let lastAssistantUuid = null;
let turnCount = 0;
let activeTurn = null;
let consumeLoopPromise = null;

function currentResumeCursor() {
  return {
    sessionId,
    resume: sessionId,
    resumeSessionAt: lastAssistantUuid,
    turnCount,
  };
}

async function startSession(command) {
  if (activeQuery) {
    activeQuery.close?.();
  }
  promptQueue?.close?.();

  sessionConfig = {
    cwd: command.cwd,
    model: command.model,
    permissionMode: command.permissionMode ?? "default",
    resume: command.resume ?? null,
    resumeSessionAt: command.resumeSessionAt ?? null,
    sessionId: command.sessionId ?? null,
  };
  sessionId = sessionConfig.resume ?? sessionConfig.sessionId ?? null;
  lastAssistantUuid = sessionConfig.resumeSessionAt ?? null;
  activeTurn = null;

  promptQueue = createPromptQueue();
  const options = {
    cwd: sessionConfig.cwd,
    model: sessionConfig.model,
    permissionMode: sessionConfig.permissionMode,
    includePartialMessages: true,
    ...(sessionConfig.resume ? { resume: sessionConfig.resume } : {}),
    ...(sessionConfig.resumeSessionAt ? { resumeSessionAt: sessionConfig.resumeSessionAt } : {}),
    ...(!sessionConfig.resume && sessionConfig.sessionId ? { sessionId: sessionConfig.sessionId } : {}),
  };

  activeQuery = query({
    prompt: promptQueue.stream(),
    options,
  });

  const initialization = await activeQuery.initializationResult();
  write({ event: "session.ready", sessionId, initialization });

  consumeLoopPromise = consumeQuery();
}

async function consumeQuery() {
  try {
    for await (const message of activeQuery) {
      if (typeof message?.session_id === "string" && message.session_id.length > 0) {
        sessionId = message.session_id;
      }
      if (typeof message?.uuid === "string" && message.uuid.length > 0) {
        lastAssistantUuid = message.uuid;
      }

      if (!activeTurn) {
        continue;
      }

      switch (message.type) {
        case "system": {
          if (message.subtype === "init") {
            write({
              event: "session.init",
              turnId: activeTurn.turnId,
              sessionId,
              payload: message,
            });
          }
          break;
        }
        case "stream_event": {
          const event = message.event ?? {};
          if (event.type === "content_block_start" && event.content_block?.type === "tool_use") {
            write({
              event: "tool.started",
              turnId: activeTurn.turnId,
              sessionId,
              toolName: event.content_block.name ?? "tool",
              payload: event.content_block,
            });
          }
          if (event.type === "content_block_delta" && event.delta?.type === "text_delta") {
            write({
              event: "delta",
              turnId: activeTurn.turnId,
              sessionId,
              text: event.delta.text ?? "",
            });
          }
          break;
        }
        case "assistant": {
          if (message.subtype === "task_progress" && message.task_id) {
            write({
              event: "task.progress",
              turnId: activeTurn.turnId,
              sessionId,
              taskId: message.task_id,
              title: message.message?.content?.[0]?.name ?? "Task",
              detail: message.message?.content?.[0]?.text ?? "",
              payload: message,
            });
          }
          const content = Array.isArray(message.message?.content) ? message.message.content : [];
          for (const block of content) {
            if (block?.type === "tool_use") {
              write({
                event: "tool.finished",
                turnId: activeTurn.turnId,
                sessionId,
                toolName: block.name ?? "tool",
                payload: block,
              });
            }
          }
          break;
        }
        case "user": {
          const content = Array.isArray(message.message?.content) ? message.message.content : [];
          for (const block of content) {
            if (block?.type === "tool_result") {
              write({
                event: "tool.output",
                turnId: activeTurn.turnId,
                sessionId,
                text:
                  typeof block.content === "string" ? block.content : JSON.stringify(block.content ?? ""),
              });
            }
          }
          break;
        }
        case "result": {
          turnCount += 1;
          write({
            event: message.is_error ? "turn.error" : "turn.completed",
            turnId: activeTurn.turnId,
            sessionId,
            result: message.result,
            error: message.is_error ? message.result : null,
            resumeCursor: currentResumeCursor(),
          });
          activeTurn = null;
          break;
        }
      }
    }

    if (activeTurn) {
      write({
        event: "runtime.error",
        turnId: activeTurn.turnId,
        sessionId,
        message: "Query closed before response received",
      });
      activeTurn = null;
    }
  } catch (error) {
    write({
      event: "runtime.error",
      turnId: activeTurn?.turnId ?? null,
      sessionId,
      message: error instanceof Error ? error.message : String(error),
    });
    activeTurn = null;
  }
}

async function sendTurn(command) {
  if (!sessionConfig || !promptQueue || !activeQuery) {
    throw new Error("Session was not started.");
  }
  if (activeTurn) {
    throw new Error("Claude runtime already has an active turn.");
  }

  activeTurn = { turnId: command.turnId };
  write({ event: "turn.started", turnId: command.turnId, sessionId });
  promptQueue.push({
    type: "user",
    session_id: sessionId ?? "",
    message: {
      role: "user",
      content: [{ type: "text", text: command.prompt }],
    },
    parent_tool_use_id: null,
  });
}

async function closeSession() {
  promptQueue?.close?.();
  activeQuery?.close?.();
  if (consumeLoopPromise) {
    await consumeLoopPromise.catch(() => {});
  }
  process.exit(0);
}

const rl = readline.createInterface({
  input: process.stdin,
  crlfDelay: Infinity,
});

for await (const line of rl) {
  const raw = line.trim();
  if (!raw) continue;
  const command = JSON.parse(raw);

  try {
    if (command.op === "start") {
      await startSession(command);
      continue;
    }
    if (command.op === "send") {
      await sendTurn(command);
      continue;
    }
    if (command.op === "interrupt") {
      await activeQuery?.interrupt?.();
      continue;
    }
    if (command.op === "close") {
      await closeSession();
    }
  } catch (error) {
    write({
      event: "runtime.error",
      turnId: command.turnId ?? null,
      sessionId,
      message: error instanceof Error ? error.message : String(error),
    });
  }
}
