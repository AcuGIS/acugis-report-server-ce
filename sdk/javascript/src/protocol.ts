import {
  PROTOCOL_SOURCE,
  PROTOCOL_VERSION,
  type ProtocolMessage,
  type ProtocolMessageType,
} from "./types.js";

export function createMessage<T = unknown>(
  type: ProtocolMessageType | string,
  payload?: T,
  id?: string,
): ProtocolMessage<T> {
  const msg: ProtocolMessage<T> = {
    source: PROTOCOL_SOURCE,
    version: PROTOCOL_VERSION,
    type,
  };
  if (payload !== undefined) {
    msg.payload = payload;
  }
  if (id !== undefined) {
    msg.id = id;
  }
  return msg;
}

export function isProtocolMessage(data: unknown): data is ProtocolMessage {
  if (!data || typeof data !== "object") {
    return false;
  }
  const msg = data as Record<string, unknown>;
  return (
    msg.source === PROTOCOL_SOURCE &&
    msg.version === PROTOCOL_VERSION &&
    typeof msg.type === "string" &&
    msg.type.length > 0
  );
}

/**
 * Validate a MessageEvent from an embedded viewer.
 * Rejects wrong origin, wrong source window, and malformed payloads.
 */
export function validateIncomingMessage(
  event: MessageEvent,
  expectedOrigin: string,
  expectedSource: Window | null | undefined,
): ProtocolMessage | null {
  if (!expectedOrigin || event.origin !== expectedOrigin) {
    return null;
  }
  if (expectedSource && event.source !== expectedSource) {
    return null;
  }
  if (!isProtocolMessage(event.data)) {
    return null;
  }
  return event.data;
}

export function serverOrigin(server: string): string {
  const url = new URL(server);
  return url.origin;
}
