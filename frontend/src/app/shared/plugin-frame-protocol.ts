/**
 * Messages a plugin view, running in a sandboxed frame, exchanges with the
 * interface. The frame has no access to the application: it can only ask the
 * host page for its context and call the handlers its plugin registered.
 */

/** Largest handler name a view may send. */
const MAXIMUM_HANDLER_LENGTH = 64;

const HANDLER_PATTERN = /^[A-Za-z][A-Za-z0-9_]*$/;

/** The view is ready and waits for its context. */
export interface ReadyMessage {
  type: 'awake:ready';
}

/** The view calls a handler of its plugin; the host answers with the same id. */
export interface InvokeMessage {
  type: 'awake:invoke';
  id: number;
  handler: string;
  payload: unknown;
}

export type ViewMessage = ReadyMessage | InvokeMessage;

/** What the host sends a view: its context, then the answers to its calls. */
export type HostMessage =
  | { type: 'awake:context'; theme: 'light' | 'dark'; kind: 'response' | 'request'; data: unknown }
  | { type: 'awake:result'; id: number; result: unknown }
  | { type: 'awake:error'; id: number; message: string };

/**
 * Reads a message from a view, or returns null for anything else: a view is
 * untrusted, so every field is checked.
 */
export function parseViewMessage(data: unknown): ViewMessage | null {
  if (typeof data !== 'object' || data === null) {
    return null;
  }
  const message = data as Record<string, unknown>;
  switch (message['type']) {
    case 'awake:ready':
      return { type: 'awake:ready' };
    case 'awake:invoke': {
      const id = message['id'];
      const handler = message['handler'];
      if (
        typeof id !== 'number' ||
        !Number.isSafeInteger(id) ||
        typeof handler !== 'string' ||
        handler.length > MAXIMUM_HANDLER_LENGTH ||
        !HANDLER_PATTERN.test(handler)
      ) {
        return null;
      }
      return { type: 'awake:invoke', id, handler, payload: message['payload'] ?? null };
    }
    default:
      return null;
  }
}

/** The theme a view should use, resolving `system` with the media query. */
export function resolvedTheme(
  preference: 'system' | 'light' | 'dark',
  prefersDark: boolean,
): 'light' | 'dark' {
  if (preference === 'system') {
    return prefersDark ? 'dark' : 'light';
  }
  return preference;
}
