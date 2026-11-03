export { AcuGIS, autoInit } from "./api.js";
export { Embed } from "./embed.js";
export {
  PROTOCOL_SOURCE,
  PROTOCOL_VERSION,
  type AuthOptions,
  type AuthType,
  type EmbedComponentOptions,
  type EmbedContentType,
  type EmbedErrorCode,
  type EmbedErrorEvent,
  type EmbedEventHandler,
  type EmbedEventMap,
  type EmbedEventType,
  type EmbedOptions,
  type ExportFormat,
  type ParameterChangeEvent,
  type ParameterValue,
  type Parameters,
  type ProtocolMessage,
  type ReportPresentation,
  type ProtocolMessageType,
  type TokenAuthOptions,
  type ViewerOptions,
} from "./types.js";
export {
  createMessage,
  isProtocolMessage,
  serverOrigin,
  validateIncomingMessage,
} from "./protocol.js";
export {
  buildComponentPath,
  buildComponentUrl,
  buildEmbedPath,
  buildEmbedUrl,
  resolveContentId,
  resolveContentType,
} from "./url.js";
export {
  assertComponentId,
  cssSize,
  COMPONENT_ID_PATTERN,
  EMBED_TOKEN_PREFIX,
  isEmbedToken,
  isPersonalAccessToken,
  normalizeAuth,
  normalizeServer,
  PAT_PREFIX,
  resolveTarget,
  serializeParameters,
} from "./options.js";

import { AcuGIS } from "./api.js";

export default AcuGIS;

// Browser IIFE global is set by tsup `globalName: "AcuGIS"`.
// Also attach when loaded as a classic script via module interop bundlers.
if (typeof window !== "undefined") {
  const w = window as Window & { AcuGIS?: typeof AcuGIS };
  if (!w.AcuGIS) {
    w.AcuGIS = AcuGIS;
  }
}
