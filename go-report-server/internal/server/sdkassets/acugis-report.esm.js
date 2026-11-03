// src/options.ts
var PAT_PREFIX = "jrs_";
var EMBED_TOKEN_PREFIX = "emb_";
function normalizeServer(server) {
  const trimmed = server.trim().replace(/\/+$/, "");
  if (!trimmed) {
    throw new Error("server URL is required");
  }
  const url = new URL(trimmed);
  if (url.protocol !== "http:" && url.protocol !== "https:") {
    throw new Error("server URL must be http or https");
  }
  return trimmed;
}
function resolveTarget(target) {
  if (typeof target !== "string") {
    if (!(target instanceof HTMLElement)) {
      throw new Error("INVALID_TARGET: target must be a selector or HTMLElement");
    }
    return target;
  }
  const el = document.querySelector(target);
  if (!(el instanceof HTMLElement)) {
    throw new Error(`INVALID_TARGET: element not found for selector ${target}`);
  }
  return el;
}
function isEmbedToken(token) {
  return token.startsWith(EMBED_TOKEN_PREFIX);
}
function isPersonalAccessToken(token) {
  return token.startsWith(PAT_PREFIX);
}
function normalizeAuth(options) {
  let token = "";
  if (options.auth) {
    if (options.auth.type !== "token") {
      throw new Error(
        'AUTH_REQUIRED: embedding requires an embed token (auth.type must be "token")'
      );
    }
    token = (options.auth.token || "").trim();
  } else if (options.token) {
    token = options.token.trim();
  }
  if (!token) {
    throw new Error(
      "AUTH_REQUIRED: embedding requires a short-lived embed token (emb_\u2026). Mint one via POST /api/v1/embed/token from your backend."
    );
  }
  if (isPersonalAccessToken(token)) {
    throw new Error(
      "AUTH_REQUIRED: personal access tokens (jrs_\u2026) cannot be used in the browser SDK. Exchange the PAT server-side for an embed token via POST /api/v1/embed/token."
    );
  }
  if (!isEmbedToken(token)) {
    throw new Error(
      "TOKEN_INVALID: expected an embed token starting with emb_. Mint one via POST /api/v1/embed/token."
    );
  }
  return { type: "token", token };
}
function cssSize(value, fallback) {
  if (value === void 0 || value === null || value === "") {
    return fallback;
  }
  if (typeof value === "number") {
    return `${value}px`;
  }
  return String(value);
}
function serializeParameters(parameters) {
  const out = {};
  if (!parameters) {
    return out;
  }
  for (const [key, value] of Object.entries(parameters)) {
    if (value === void 0 || value === null) {
      continue;
    }
    if (Array.isArray(value)) {
      out[key] = value.map((v) => String(v)).join(",");
    } else {
      out[key] = String(value);
    }
  }
  return out;
}
var COMPONENT_ID_PATTERN = /^[a-z][a-z0-9_-]{0,63}$/;
function assertComponentId(component) {
  if (typeof component !== "string" || !COMPONENT_ID_PATTERN.test(component)) {
    throw new Error(
      "INVALID_PARAMETER: component must match ^[a-z][a-z0-9_-]{0,63}$"
    );
  }
  return component;
}
function defaultViewerOptions(viewer) {
  return {
    toolbar: viewer?.toolbar !== false,
    parameters: viewer?.parameters !== false,
    export: viewer?.export !== false,
    pagination: viewer?.pagination !== false,
    zoom: viewer?.zoom !== false,
    close: viewer?.close === true
  };
}

// src/types.ts
var PROTOCOL_SOURCE = "acugis-report";
var PROTOCOL_VERSION = 1;

// src/protocol.ts
function createMessage(type, payload, id) {
  const msg = {
    source: PROTOCOL_SOURCE,
    version: PROTOCOL_VERSION,
    type
  };
  if (payload !== void 0) {
    msg.payload = payload;
  }
  if (id !== void 0) {
    msg.id = id;
  }
  return msg;
}
function isProtocolMessage(data) {
  if (!data || typeof data !== "object") {
    return false;
  }
  const msg = data;
  return msg.source === PROTOCOL_SOURCE && msg.version === PROTOCOL_VERSION && typeof msg.type === "string" && msg.type.length > 0;
}
function validateIncomingMessage(event, expectedOrigin, expectedSource) {
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
function serverOrigin(server) {
  const url = new URL(server);
  return url.origin;
}

// src/url.ts
function resolveContentType(options) {
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
    "content type is required (type, reportId, report, dashboardId, viewId, or mapReportId)"
  );
}
function resolveContentId(options, type) {
  const fromTyped = type === "report" ? options.reportId || options.report : type === "dashboard" ? options.dashboardId : type === "view" ? options.viewId : options.mapReportId;
  const id = (fromTyped || options.id || "").trim();
  if (!id) {
    throw new Error("content id is required");
  }
  return id;
}
function buildEmbedPath(type, id) {
  const encoded = id.split("/").map((part) => encodeURIComponent(part)).join("/");
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
      const _exhaustive = type;
      return _exhaustive;
    }
  }
}
function buildEmbedUrl(input) {
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
    url.searchParams.set("et", embedToken);
    const params2 = serializeParameters(input.parameters);
    for (const [k, v] of Object.entries(params2)) {
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
function buildComponentPath(report, component) {
  const encodedReport = report.split("/").map((part) => encodeURIComponent(part)).join("/");
  return `/embed/report/${encodedReport}/component/${encodeURIComponent(component)}`;
}
function buildComponentUrl(input) {
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
function requireEmbedToken(embedToken) {
  if (!embedToken) {
    throw new Error("AUTH_REQUIRED: embedToken is required");
  }
  if (embedToken.startsWith("jrs_")) {
    throw new Error(
      "AUTH_REQUIRED: personal access tokens cannot be placed in embed URLs"
    );
  }
  if (!embedToken.startsWith("emb_")) {
    throw new Error("TOKEN_INVALID: embedToken must start with emb_");
  }
  return embedToken;
}
function isComponentReservedParameter(name) {
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

// src/embed.ts
var instanceSeq = 0;
var Embed = class {
  constructor(options, componentId) {
    this.destroyed = false;
    this.readyResolved = false;
    this.listeners = {};
    this.iframeWindow = null;
    if (componentId !== void 0) {
      assertComponentId(componentId);
    }
    this.id = `acugis-embed-${++instanceSeq}`;
    this.server = normalizeServer(options.server);
    this.serverOrigin = serverOrigin(this.server);
    this.type = resolveContentType(options);
    this.contentId = resolveContentId(options, this.type);
    this.auth = normalizeAuth(options);
    this.parameters = { ...options.parameters || {} };
    this.hostOrigin = options.origin || (typeof window !== "undefined" ? window.location.origin : this.serverOrigin);
    this.container = resolveTarget(options.target);
    this.readyPromise = new Promise((resolve, reject) => {
      this.resolveReady = resolve;
      this.rejectReady = reject;
    });
    this.readyPromise.catch(() => void 0);
    this.iframe = document.createElement("iframe");
    this.iframe.setAttribute("title", `AcuGIS ${this.type}`);
    this.iframe.setAttribute("data-acugis-embed-id", this.id);
    this.iframe.setAttribute(
      "sandbox",
      "allow-scripts allow-same-origin allow-forms allow-downloads allow-popups"
    );
    this.componentId = componentId ?? "";
    this.componentPresentation = componentId !== void 0;
    this.contentPresentation = options.presentation === "content";
    this.autoWidth = this.componentPresentation && (options.width === void 0 || options.width === "auto");
    this.autoHeight = this.componentPresentation ? options.height === void 0 || options.height === "auto" : this.contentPresentation && (options.autoResize === true || options.height === "auto");
    this.iframe.style.border = "0";
    this.iframe.style.display = "block";
    this.iframe.style.width = this.componentPresentation ? this.autoWidth ? "0px" : cssSize(options.width, "0px") : cssSize(options.width, "100%");
    this.iframe.style.height = this.componentPresentation ? this.autoHeight ? "0px" : cssSize(options.height, "0px") : this.contentPresentation && options.height === "auto" ? "0px" : cssSize(options.height, "800px");
    const { url } = this.componentPresentation ? buildComponentUrl({
      server: this.server,
      report: this.contentId,
      component: this.componentId,
      parameters: this.parameters,
      embedToken: this.auth.token
    }) : buildEmbedUrl({
      server: this.server,
      type: this.type,
      id: this.contentId,
      parameters: this.parameters,
      viewer: options.viewer,
      datasource: options.datasource,
      embedToken: this.auth.token,
      autoResize: options.autoResize,
      presentation: options.presentation
    });
    this.onMessageBound = (event) => this.handleMessage(event);
    window.addEventListener("message", this.onMessageBound);
    this.iframe.addEventListener("load", () => {
      this.iframeWindow = this.iframe.contentWindow;
      this.post("auth", { type: "token", token: this.auth.token });
      this.post("configure", {
        hostOrigin: this.hostOrigin,
        viewer: options.viewer,
        autoResize: !!options.autoResize,
        parameters: this.parameters
      });
    });
    this.container.replaceChildren(this.iframe);
    this.iframe.src = url;
  }
  ready() {
    this.assertAlive();
    return this.readyPromise;
  }
  on(event, callback) {
    this.assertAlive();
    if (!this.listeners[event]) {
      this.listeners[event] = /* @__PURE__ */ new Set();
    }
    this.listeners[event].add(callback);
    return this;
  }
  off(event, callback) {
    const set = this.listeners[event];
    set?.delete(callback);
    return this;
  }
  setParameters(parameters) {
    this.assertAlive();
    this.parameters = { ...this.parameters, ...parameters };
    this.post("setParameters", { parameters: this.parameters });
    return this;
  }
  getParameters() {
    return { ...this.parameters };
  }
  refresh() {
    this.assertAlive();
    this.post("refresh");
    return this;
  }
  resize() {
    this.assertAlive();
    this.post("resize");
    return this;
  }
  setHeight(height) {
    this.assertAlive();
    this.iframe.style.height = cssSize(height, this.iframe.style.height || "800px");
    this.post("setHeight", { height: this.iframe.style.height });
    return this;
  }
  setWidth(width) {
    this.assertAlive();
    this.iframe.style.width = cssSize(width, this.iframe.style.width || "100%");
    this.post("setWidth", { width: this.iframe.style.width });
    return this;
  }
  nextPage() {
    this.assertAlive();
    this.post("nextPage");
    return this;
  }
  previousPage() {
    this.assertAlive();
    this.post("previousPage");
    return this;
  }
  goToPage(page) {
    this.assertAlive();
    this.post("goToPage", { page });
    return this;
  }
  zoomIn() {
    this.assertAlive();
    this.post("zoomIn");
    return this;
  }
  zoomOut() {
    this.assertAlive();
    this.post("zoomOut");
    return this;
  }
  setZoom(percent) {
    this.assertAlive();
    this.post("setZoom", { percent });
    return this;
  }
  export(format) {
    this.assertAlive();
    if (this.componentPresentation) {
      throw new Error("EXPORT_UNSUPPORTED: component embeds do not support export");
    }
    this.post("export", { format });
    return this;
  }
  destroy() {
    if (this.destroyed) {
      return;
    }
    this.destroyed = true;
    window.removeEventListener("message", this.onMessageBound);
    this.iframe.remove();
    for (const key of Object.keys(this.listeners)) {
      this.listeners[key]?.clear();
    }
    if (!this.readyResolved) {
      this.rejectReady(new Error("DESTROYED: embed destroyed before ready"));
    }
  }
  assertAlive() {
    if (this.destroyed) {
      throw new Error("DESTROYED: embed instance has been destroyed");
    }
  }
  post(type, payload) {
    const win = this.iframeWindow || this.iframe.contentWindow;
    if (!win) {
      return;
    }
    win.postMessage(createMessage(type, payload), this.serverOrigin);
  }
  handleMessage(event) {
    if (this.destroyed) {
      return;
    }
    const expectedSource = this.iframeWindow || this.iframe.contentWindow;
    const msg = validateIncomingMessage(event, this.serverOrigin, expectedSource);
    if (!msg) {
      return;
    }
    this.dispatchProtocol(msg);
  }
  dispatchProtocol(msg) {
    switch (msg.type) {
      case "ready":
      case "load":
        if (!this.readyResolved) {
          this.readyResolved = true;
          this.resolveReady();
        }
        this.emit("load", {});
        break;
      case "error": {
        const payload = msg.payload || {};
        const err = {
          code: payload.code || "SERVER_ERROR",
          message: payload.message || "Embedded viewer error"
        };
        if (!this.readyResolved) {
          this.readyResolved = true;
          this.rejectReady(new Error(`${err.code}: ${err.message}`));
        }
        this.emit("error", err);
        break;
      }
      case "parameterChange":
      case "parameters": {
        const payload = msg.payload || {};
        if (payload.parameters) {
          this.parameters = { ...payload.parameters };
        }
        this.emit("parameterChange", { parameters: this.getParameters() });
        break;
      }
      case "pageChange":
        this.emit("pageChange", msg.payload || {});
        break;
      case "selection":
        this.emit("selection", msg.payload || {});
        break;
      case "filterChange":
        this.emit("filterChange", msg.payload || {});
        break;
      case "refresh":
        this.emit("refresh", {});
        break;
      case "export":
        this.emit("export", msg.payload || {});
        break;
      case "resize": {
        const payload = msg.payload || {};
        if (this.componentPresentation) {
          if (this.autoWidth && typeof payload.width === "number" && payload.width > 0) {
            this.iframe.style.width = `${payload.width}px`;
          }
          if (this.autoHeight && typeof payload.height === "number" && payload.height > 0) {
            this.iframe.style.height = `${payload.height}px`;
          }
        } else if (!this.contentPresentation) {
          if (typeof payload.height === "number" && payload.height > 0) {
            this.iframe.style.height = `${payload.height}px`;
          }
          if (typeof payload.width === "number" && payload.width > 0) {
            this.iframe.style.width = `${payload.width}px`;
          }
        } else if (this.autoHeight && typeof payload.height === "number" && payload.height > 0) {
          this.iframe.style.height = `${payload.height}px`;
        }
        this.emit("resize", {
          width: payload.width || this.iframe.clientWidth,
          height: payload.height || this.iframe.clientHeight
        });
        break;
      }
      default:
        break;
    }
  }
  emit(event, payload) {
    const set = this.listeners[event];
    if (!set) {
      return;
    }
    for (const cb of set) {
      try {
        cb(payload);
      } catch {
      }
    }
  }
};

// src/api.ts
function withType(options, type, idField) {
  const id = options[idField] || options.id;
  return {
    ...options,
    type,
    id
  };
}
function autoInit(root = document) {
  const nodes = root.querySelectorAll(
    ".acugis-report, .acugis-dashboard, .acugis-view, .acugis-map-report"
  );
  const embeds = [];
  nodes.forEach((el) => {
    if (el.getAttribute("data-acugis-initialized") === "1") {
      return;
    }
    const server = el.getAttribute("data-server") || "";
    const token = el.getAttribute("data-token") || "";
    if (!server || !token) {
      return;
    }
    let options = { server, target: el, token };
    if (el.classList.contains("acugis-dashboard") || el.dataset.dashboardId) {
      options = {
        ...options,
        type: "dashboard",
        dashboardId: el.dataset.dashboardId || el.getAttribute("data-dashboard-id") || void 0
      };
    } else if (el.classList.contains("acugis-view") || el.dataset.viewId) {
      options = {
        ...options,
        type: "view",
        viewId: el.dataset.viewId || el.getAttribute("data-view-id") || void 0
      };
    } else if (el.classList.contains("acugis-map-report") || el.dataset.mapReportId) {
      options = {
        ...options,
        type: "map-report",
        mapReportId: el.dataset.mapReportId || el.getAttribute("data-map-report-id") || void 0
      };
    } else {
      options = {
        ...options,
        type: "report",
        reportId: el.dataset.reportId || el.getAttribute("data-report-id") || void 0
      };
    }
    try {
      const embed = new Embed(options);
      el.setAttribute("data-acugis-initialized", "1");
      embeds.push(embed);
    } catch {
    }
  });
  return embeds;
}
var AcuGIS = {
  embed(options) {
    return new Embed(options);
  },
  embedReport(options) {
    const reportId = options.reportId || options.report;
    return new Embed(withType({ ...options, reportId }, "report", "reportId"));
  },
  embedComponent(options) {
    return new Embed(
      {
        server: options.server,
        target: options.target,
        type: "report",
        report: options.report,
        reportId: options.report,
        auth: options.auth,
        parameters: options.parameters,
        width: options.width,
        height: options.height,
        origin: options.origin
      },
      options.component
    );
  },
  embedDashboard(options) {
    return new Embed(withType(options, "dashboard", "dashboardId"));
  },
  embedView(options) {
    return new Embed(withType(options, "view", "viewId"));
  },
  embedMapReport(options) {
    return new Embed(withType(options, "map-report", "mapReportId"));
  },
  autoInit
};

// src/index.ts
var src_default = AcuGIS;
if (typeof window !== "undefined") {
  const w = window;
  if (!w.AcuGIS) {
    w.AcuGIS = AcuGIS;
  }
}
export {
  AcuGIS,
  COMPONENT_ID_PATTERN,
  EMBED_TOKEN_PREFIX,
  Embed,
  PAT_PREFIX,
  PROTOCOL_SOURCE,
  PROTOCOL_VERSION,
  assertComponentId,
  autoInit,
  buildComponentPath,
  buildComponentUrl,
  buildEmbedPath,
  buildEmbedUrl,
  createMessage,
  cssSize,
  src_default as default,
  isEmbedToken,
  isPersonalAccessToken,
  isProtocolMessage,
  normalizeAuth,
  normalizeServer,
  resolveContentId,
  resolveContentType,
  resolveTarget,
  serializeParameters,
  serverOrigin,
  validateIncomingMessage
};
//# sourceMappingURL=acugis-report.esm.js.map