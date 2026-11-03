import type {
  EmbedContentType,
  EmbedOptions,
  Parameters,
  ReportPresentation,
  ViewerOptions,
} from "./types.js";
import {
  assertComponentId,
  defaultViewerOptions,
  serializeParameters,
} from "./options.js";

export function resolveContentType(options: EmbedOptions): EmbedContentType {
  if (options.type) {
    return options.type;
  }
  if (options.reportId || options.report) {
    return "report";
  }
  if (options.dashboardId) {
    return "dashboard";
  }
  if (options.viewId) {
    return "view";
  }
  if (options.mapReportId) {
    return "map-report";
  }
  throw new Error(
    "content type is required (type, reportId, report, dashboardId, viewId, or mapReportId)",
  );
}

export function resolveContentId(options: EmbedOptions, type: EmbedContentType): string {
  const fromTyped =
    type === "report"
      ? options.reportId || options.report
      : type === "dashboard"
        ? options.dashboardId
        : type === "view"
          ? options.viewId
          : options.mapReportId;
  const id = (fromTyped || options.id || "").trim();
  if (!id) {
    throw new Error("content id is required");
  }
  return id;
}

export function buildEmbedPath(type: EmbedContentType, id: string): string {
  const encoded = id
    .split("/")
    .map((part) => encodeURIComponent(part))
    .join("/");
  switch (type) {
    case "report":
      return `/embed/report/${encoded}`;
    case "dashboard":
      return `/embed/dashboard/${encoded}`;
    case "view":
      return `/embed/view/${encoded}`;
    case "map-report":
      return `/embed/map-report/${encoded}`;
    default: {
      const _exhaustive: never = type;
      return _exhaustive;
    }
  }
}

export interface EmbedUrlParts {
  url: string;
  origin: string;
}

export function buildEmbedUrl(input: {
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
}): EmbedUrlParts {
  const base = new URL(input.server);
  const pathBase = base.pathname.replace(/\/+$/, "");
  let embedPath = buildEmbedPath(input.type, input.id);
  const contentReport = input.type === "report" && input.presentation === "content";
  if (contentReport) {
    embedPath += "/content";
  }
  const url = new URL(`${pathBase}${embedPath}` || embedPath, base.origin);
  const embedToken = requireEmbedToken(input.embedToken);

  if (contentReport) {
    // Chrome flags belong to the viewer. Content rendering pins identity and
    // format on the server, so those names are not copied from host parameters.
    url.searchParams.set("et", embedToken);
    const params = serializeParameters(input.parameters);
    for (const [k, v] of Object.entries(params)) {
      if (k === "et" || k === "_repName" || k === "_repFormat" || k === "report") {
        continue;
      }
      url.searchParams.set(k, v);
    }
    return { url: url.toString(), origin: base.origin };
  }

  const viewer = defaultViewerOptions(input.viewer);
  url.searchParams.set("embed", "1");
  if (!viewer.toolbar) url.searchParams.set("toolbar", "0");
  if (!viewer.parameters) url.searchParams.set("parameters", "0");
  if (!viewer.export) url.searchParams.set("export", "0");
  if (!viewer.pagination) url.searchParams.set("pagination", "0");
  if (!viewer.zoom) url.searchParams.set("zoom", "0");
  if (viewer.close) url.searchParams.set("close", "1");
  if (input.autoResize) url.searchParams.set("autoResize", "1");
  if (input.datasource) url.searchParams.set("datasource", input.datasource);

  const params = serializeParameters(input.parameters);
  for (const [k, v] of Object.entries(params)) {
    url.searchParams.set(k, v);
  }
  url.searchParams.set("et", embedToken);

  return { url: url.toString(), origin: base.origin };
}

export function buildComponentPath(report: string, component: string): string {
  const encodedReport = report
    .split("/")
    .map((part) => encodeURIComponent(part))
    .join("/");
  return `/embed/report/${encodedReport}/component/${encodeURIComponent(component)}`;
}

/**
 * Chrome-less component document.
 * Parameter serialization matches content-mode embedReport, and server
 * controls are left off the query. The path component id is authoritative.
 */
export function buildComponentUrl(input: {
  server: string;
  report: string;
  component: string;
  parameters?: Parameters;
  /** Short-lived embed token only — never long-lived secrets. */
  embedToken?: string;
}): EmbedUrlParts {
  const component = assertComponentId(input.component);
  const embedToken = requireEmbedToken(input.embedToken);
  const base = new URL(input.server);
  const pathBase = base.pathname.replace(/\/+$/, "");
  const embedPath = buildComponentPath(input.report, component);
  const url = new URL(`${pathBase}${embedPath}` || embedPath, base.origin);

  url.searchParams.set("et", embedToken);
  const params = serializeParameters(input.parameters);
  for (const [k, v] of Object.entries(params)) {
    if (isComponentReservedParameter(k)) {
      continue;
    }
    url.searchParams.set(k, v);
  }
  return { url: url.toString(), origin: base.origin };
}

function requireEmbedToken(embedToken: string | undefined): string {
  // Embed tokens are required for /embed/* routes. Pass as short-lived `et`.
  if (!embedToken) {
    throw new Error("AUTH_REQUIRED: embedToken is required");
  }
  if (embedToken.startsWith("jrs_")) {
    throw new Error(
      "AUTH_REQUIRED: personal access tokens cannot be placed in embed URLs",
    );
  }
  if (!embedToken.startsWith("emb_")) {
    throw new Error("TOKEN_INVALID: embedToken must start with emb_");
  }
  return embedToken;
}

function isComponentReservedParameter(name: string): boolean {
  switch (name) {
    case "et":
    case "_repName":
    case "_repFormat":
    case "report":
    case "_componentId":
    case "_dataSource":
    case "datasource":
      return true;
    default:
      return false;
  }
}
