/** Content types supported by the Embedding SDK. */
export type EmbedContentType = "report" | "dashboard" | "view" | "map-report";

/**
 * Full-report presentation.
 * Omit or "viewer" for the Report Server viewer.
 * "content" returns the Jasper HTML report directly, with no viewer chrome.
 */
export type ReportPresentation = "viewer" | "content";

/** Export formats supported by Report Server execute. */
export type ExportFormat =
  | "pdf"
  | "xlsx"
  | "xls"
  | "csv"
  | "docx"
  | "rtf"
  | "odt"
  | "ods"
  | "pptx"
  | "xml"
  | "html";

export type AuthType = "token";

/**
 * Embed authentication. Embed tokens (`emb_…`) are REQUIRED.
 * Mint them server-side via POST /api/v1/embed/token using a session or PAT.
 * Personal access tokens (`jrs_…`) and session cookies are not accepted by the SDK.
 */
export interface TokenAuthOptions {
  type: "token";
  /** Short-lived embed token (`emb_…`) from POST /api/v1/embed/token. */
  token: string;
}

export type AuthOptions = TokenAuthOptions;

export interface ViewerOptions {
  toolbar?: boolean;
  parameters?: boolean;
  export?: boolean;
  pagination?: boolean;
  zoom?: boolean;
  close?: boolean;
}

export type ParameterValue = string | number | boolean | null | Array<string | number | boolean>;

export type Parameters = Record<string, ParameterValue>;

export interface EmbedOptions {
  server: string;
  target: string | HTMLElement;
  /** Content id/key. Prefer typed convenience fields when using embedReport etc. */
  id?: string;
  type?: EmbedContentType;
  reportId?: string;
  /** Report key accepted by embedReport. Same role as reportId. */
  report?: string;
  /**
   * Full-report presentation. Omit or "viewer" to keep the Report Server viewer.
   * "content" is explicit opt-in for chrome-less Jasper HTML.
   */
  presentation?: ReportPresentation;
  dashboardId?: string;
  viewId?: string;
  mapReportId?: string;
  /**
   * Required embed auth. Prefer auth: { type: "token", token }.
   * Convenience: pass token: "emb_…" at the top level.
   */
  auth?: AuthOptions;
  /** Required short-lived embed token (`emb_…`). Alias for auth.token. */
  token?: string;
  parameters?: Parameters;
  width?: string | number;
  height?: string | number;
  autoResize?: boolean;
  viewer?: ViewerOptions;
  datasource?: string;
  /** Host origin override for postMessage validation (defaults to window.location.origin). */
  origin?: string;
}

/**
 * One stable report component.
 * Width and height default to "auto", which follows the component's natural
 * .jrPage box. An explicit size is left as given.
 */
export interface EmbedComponentOptions {
  server: string;
  report: string;
  /** Designer Embed ID. Case-sensitive. Not derived from the report key. */
  component: string;
  target: string | HTMLElement;
  auth: AuthOptions;
  parameters?: Parameters;
  width?: string | number;
  height?: string | number;
  /** Host origin override. Same role as EmbedOptions.origin. */
  origin?: string;
}

export type EmbedEventType =
  | "load"
  | "error"
  | "parameterChange"
  | "pageChange"
  | "selection"
  | "filterChange"
  | "refresh"
  | "export"
  | "resize";

export type EmbedErrorCode =
  | "AUTH_REQUIRED"
  | "ACCESS_DENIED"
  | "CONTENT_NOT_FOUND"
  | "INVALID_PARAMETER"
  | "SERVER_ERROR"
  | "EMBED_ORIGIN_DENIED"
  | "TOKEN_EXPIRED"
  | "TOKEN_INVALID"
  | "INVALID_TARGET"
  | "DESTROYED"
  | "PROTOCOL_ERROR";

export interface EmbedErrorEvent {
  code: EmbedErrorCode | string;
  message: string;
}

export interface ParameterChangeEvent {
  parameters: Parameters;
}

export interface PageChangeEvent {
  page: number;
  pageCount?: number;
}

export interface SelectionEvent {
  features?: unknown[];
  [key: string]: unknown;
}

export interface ResizeEvent {
  width: number;
  height: number;
}

export interface ExportEvent {
  format: ExportFormat | string;
}

export interface EmbedEventMap {
  load: Record<string, never>;
  error: EmbedErrorEvent;
  parameterChange: ParameterChangeEvent;
  pageChange: PageChangeEvent;
  selection: SelectionEvent;
  filterChange: Record<string, unknown>;
  refresh: Record<string, never>;
  export: ExportEvent;
  resize: ResizeEvent;
}

export type EmbedEventHandler<T extends EmbedEventType> = (
  event: EmbedEventMap[T],
) => void;

/** Wire protocol between host SDK and embedded viewer. */
export const PROTOCOL_SOURCE = "acugis-report" as const;
export const PROTOCOL_VERSION = 1 as const;

export type ProtocolMessageType =
  | "ready"
  | "load"
  | "error"
  | "setParameters"
  | "getParameters"
  | "parameters"
  | "parameterChange"
  | "refresh"
  | "export"
  | "resize"
  | "setHeight"
  | "setWidth"
  | "nextPage"
  | "previousPage"
  | "goToPage"
  | "zoomIn"
  | "zoomOut"
  | "setZoom"
  | "pageChange"
  | "selection"
  | "filterChange"
  | "configure"
  | "auth"
  | "ping"
  | "pong";

export interface ProtocolMessage<T = unknown> {
  source: typeof PROTOCOL_SOURCE;
  version: typeof PROTOCOL_VERSION;
  type: ProtocolMessageType | string;
  id?: string;
  payload?: T;
}
