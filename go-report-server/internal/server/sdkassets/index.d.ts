/** Content types supported by the Embedding SDK. */
type EmbedContentType = "report" | "dashboard" | "view" | "map-report";
/**
 * Full-report presentation.
 * Omit or "viewer" for the Report Server viewer.
 * "content" returns the Jasper HTML report directly, with no viewer chrome.
 */
type ReportPresentation = "viewer" | "content";
/** Export formats supported by Report Server execute. */
type ExportFormat = "pdf" | "xlsx" | "xls" | "csv" | "docx" | "rtf" | "odt" | "ods" | "pptx" | "xml" | "html";
type AuthType = "token";
/**
 * Embed authentication. Embed tokens (`emb_…`) are REQUIRED.
 * Mint them server-side via POST /api/v1/embed/token using a session or PAT.
 * Personal access tokens (`jrs_…`) and session cookies are not accepted by the SDK.
 */
interface TokenAuthOptions {
    type: "token";
    /** Short-lived embed token (`emb_…`) from POST /api/v1/embed/token. */
    token: string;
}
type AuthOptions = TokenAuthOptions;
interface ViewerOptions {
    toolbar?: boolean;
    parameters?: boolean;
    export?: boolean;
    pagination?: boolean;
    zoom?: boolean;
    close?: boolean;
}
type ParameterValue = string | number | boolean | null | Array<string | number | boolean>;
type Parameters = Record<string, ParameterValue>;
interface EmbedOptions {
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
interface EmbedComponentOptions {
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
type EmbedEventType = "load" | "error" | "parameterChange" | "pageChange" | "selection" | "filterChange" | "refresh" | "export" | "resize";
type EmbedErrorCode = "AUTH_REQUIRED" | "ACCESS_DENIED" | "CONTENT_NOT_FOUND" | "INVALID_PARAMETER" | "SERVER_ERROR" | "EMBED_ORIGIN_DENIED" | "TOKEN_EXPIRED" | "TOKEN_INVALID" | "INVALID_TARGET" | "DESTROYED" | "PROTOCOL_ERROR";
interface EmbedErrorEvent {
    code: EmbedErrorCode | string;
    message: string;
}
interface ParameterChangeEvent {
    parameters: Parameters;
}
interface PageChangeEvent {
    page: number;
    pageCount?: number;
}
interface SelectionEvent {
    features?: unknown[];
    [key: string]: unknown;
}
interface ResizeEvent {
    width: number;
    height: number;
}
interface ExportEvent {
    format: ExportFormat | string;
}
interface EmbedEventMap {
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
type EmbedEventHandler<T extends EmbedEventType> = (event: EmbedEventMap[T]) => void;
/** Wire protocol between host SDK and embedded viewer. */
declare const PROTOCOL_SOURCE: "acugis-report";
declare const PROTOCOL_VERSION: 1;
type ProtocolMessageType = "ready" | "load" | "error" | "setParameters" | "getParameters" | "parameters" | "parameterChange" | "refresh" | "export" | "resize" | "setHeight" | "setWidth" | "nextPage" | "previousPage" | "goToPage" | "zoomIn" | "zoomOut" | "setZoom" | "pageChange" | "selection" | "filterChange" | "configure" | "auth" | "ping" | "pong";
interface ProtocolMessage<T = unknown> {
    source: typeof PROTOCOL_SOURCE;
    version: typeof PROTOCOL_VERSION;
    type: ProtocolMessageType | string;
    id?: string;
    payload?: T;
}

declare class Embed {
    readonly id: string;
    readonly type: EmbedContentType;
    readonly contentId: string;
    readonly server: string;
    readonly serverOrigin: string;
    private readonly container;
    private readonly iframe;
    private readonly auth;
    private readonly hostOrigin;
    private parameters;
    private destroyed;
    private readyResolved;
    private readonly readyPromise;
    private resolveReady;
    private rejectReady;
    private readonly listeners;
    private readonly onMessageBound;
    private iframeWindow;
    private readonly componentId;
    private readonly componentPresentation;
    private readonly contentPresentation;
    private readonly autoWidth;
    private readonly autoHeight;
    constructor(options: EmbedOptions, componentId?: string);
    ready(): Promise<void>;
    on<T extends EmbedEventType>(event: T, callback: EmbedEventHandler<T>): this;
    off<T extends EmbedEventType>(event: T, callback: EmbedEventHandler<T>): this;
    setParameters(parameters: Parameters): this;
    getParameters(): Parameters;
    refresh(): this;
    resize(): this;
    setHeight(height: string | number): this;
    setWidth(width: string | number): this;
    nextPage(): this;
    previousPage(): this;
    goToPage(page: number): this;
    zoomIn(): this;
    zoomOut(): this;
    setZoom(percent: number): this;
    export(format: ExportFormat | string): this;
    destroy(): void;
    private assertAlive;
    private post;
    private handleMessage;
    private dispatchProtocol;
    private emit;
}

/** Auto-initialize declarative embeds: <div class="acugis-report" data-report-id="…"> */
declare function autoInit(root?: ParentNode): Embed[];
declare const AcuGIS: {
    embed(options: EmbedOptions): Embed;
    embedReport(options: EmbedOptions): Embed;
    embedComponent(options: EmbedComponentOptions): Embed;
    embedDashboard(options: EmbedOptions): Embed;
    embedView(options: EmbedOptions): Embed;
    embedMapReport(options: EmbedOptions): Embed;
    autoInit: typeof autoInit;
};

declare function createMessage<T = unknown>(type: ProtocolMessageType | string, payload?: T, id?: string): ProtocolMessage<T>;
declare function isProtocolMessage(data: unknown): data is ProtocolMessage;
/**
 * Validate a MessageEvent from an embedded viewer.
 * Rejects wrong origin, wrong source window, and malformed payloads.
 */
declare function validateIncomingMessage(event: MessageEvent, expectedOrigin: string, expectedSource: Window | null | undefined): ProtocolMessage | null;
declare function serverOrigin(server: string): string;

declare function resolveContentType(options: EmbedOptions): EmbedContentType;
declare function resolveContentId(options: EmbedOptions, type: EmbedContentType): string;
declare function buildEmbedPath(type: EmbedContentType, id: string): string;
interface EmbedUrlParts {
    url: string;
    origin: string;
}
declare function buildEmbedUrl(input: {
    server: string;
    type: EmbedContentType;
    id: string;
    parameters?: Parameters;
    viewer?: ViewerOptions;
    datasource?: string;
    /** Short-lived embed token only — never long-lived secrets. */
    embedToken?: string;
    autoResize?: boolean;
    presentation?: ReportPresentation;
}): EmbedUrlParts;
declare function buildComponentPath(report: string, component: string): string;
/**
 * Chrome-less component document.
 * Parameter serialization matches content-mode embedReport, and server
 * controls are left off the query. The path component id is authoritative.
 */
declare function buildComponentUrl(input: {
    server: string;
    report: string;
    component: string;
    parameters?: Parameters;
    /** Short-lived embed token only — never long-lived secrets. */
    embedToken?: string;
}): EmbedUrlParts;

/** Personal Access Token prefix — never accepted by the Embedding SDK. */
declare const PAT_PREFIX = "jrs_";
/** Short-lived embed token prefix issued by POST /api/v1/embed/token. */
declare const EMBED_TOKEN_PREFIX = "emb_";
declare function normalizeServer(server: string): string;
declare function resolveTarget(target: string | HTMLElement): HTMLElement;
declare function isEmbedToken(token: string): boolean;
declare function isPersonalAccessToken(token: string): boolean;
/**
 * Resolve and validate embed auth.
 * Embed tokens are mandatory — session auth and PATs are rejected.
 */
declare function normalizeAuth(options: EmbedOptions): AuthOptions;
declare function cssSize(value: string | number | undefined, fallback: string): string;
declare function serializeParameters(parameters: Record<string, unknown> | undefined): Record<string, string>;
/**
 * Designer Embed ID. Values are matched as given: not trimmed into another
 * id and not lowercased.
 */
declare const COMPONENT_ID_PATTERN: RegExp;
declare function assertComponentId(component: string): string;

export { AcuGIS, type AuthOptions, type AuthType, COMPONENT_ID_PATTERN, EMBED_TOKEN_PREFIX, Embed, type EmbedComponentOptions, type EmbedContentType, type EmbedErrorCode, type EmbedErrorEvent, type EmbedEventHandler, type EmbedEventMap, type EmbedEventType, type EmbedOptions, type ExportFormat, PAT_PREFIX, PROTOCOL_SOURCE, PROTOCOL_VERSION, type ParameterChangeEvent, type ParameterValue, type Parameters, type ProtocolMessage, type ProtocolMessageType, type ReportPresentation, type TokenAuthOptions, type ViewerOptions, assertComponentId, autoInit, buildComponentPath, buildComponentUrl, buildEmbedPath, buildEmbedUrl, createMessage, cssSize, AcuGIS as default, isEmbedToken, isPersonalAccessToken, isProtocolMessage, normalizeAuth, normalizeServer, resolveContentId, resolveContentType, resolveTarget, serializeParameters, serverOrigin, validateIncomingMessage };
