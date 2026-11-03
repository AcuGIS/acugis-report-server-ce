import {
  assertComponentId,
  cssSize,
  normalizeAuth,
  normalizeServer,
  resolveTarget,
} from "./options.js";
import { createMessage, serverOrigin, validateIncomingMessage } from "./protocol.js";
import {
  buildComponentUrl,
  buildEmbedUrl,
  resolveContentId,
  resolveContentType,
} from "./url.js";
import type {
  EmbedContentType,
  EmbedErrorEvent,
  EmbedEventHandler,
  EmbedEventMap,
  EmbedEventType,
  EmbedOptions,
  ExportFormat,
  Parameters,
  ProtocolMessage,
} from "./types.js";

type ListenerMap = {
  [K in EmbedEventType]?: Set<EmbedEventHandler<K>>;
};

let instanceSeq = 0;

export class Embed {
  readonly id: string;
  readonly type: EmbedContentType;
  readonly contentId: string;
  readonly server: string;
  readonly serverOrigin: string;

  private readonly container: HTMLElement;
  private readonly iframe: HTMLIFrameElement;
  private readonly auth: ReturnType<typeof normalizeAuth>;
  private readonly hostOrigin: string;
  private parameters: Parameters;
  private destroyed = false;
  private readyResolved = false;
  private readonly readyPromise: Promise<void>;
  private resolveReady!: () => void;
  private rejectReady!: (err: Error) => void;
  private readonly listeners: ListenerMap = {};
  private readonly onMessageBound: (event: MessageEvent) => void;
  private iframeWindow: Window | null = null;
  private readonly componentId: string;
  private readonly componentPresentation: boolean;
  private readonly contentPresentation: boolean;
  private readonly autoWidth: boolean;
  private readonly autoHeight: boolean;

  constructor(options: EmbedOptions, componentId?: string) {
    if (componentId !== undefined) {
      assertComponentId(componentId);
    }
    this.id = `acugis-embed-${++instanceSeq}`;
    this.server = normalizeServer(options.server);
    this.serverOrigin = serverOrigin(this.server);
    this.type = resolveContentType(options);
    this.contentId = resolveContentId(options, this.type);
    this.auth = normalizeAuth(options);
    this.parameters = { ...(options.parameters || {}) };
    this.hostOrigin =
      options.origin ||
      (typeof window !== "undefined" ? window.location.origin : this.serverOrigin);

    this.container = resolveTarget(options.target);
    this.readyPromise = new Promise<void>((resolve, reject) => {
      this.resolveReady = resolve;
      this.rejectReady = reject;
    });
    // Prevent unhandled rejection if destroy() before ready
    this.readyPromise.catch(() => undefined);

    this.iframe = document.createElement("iframe");
    this.iframe.setAttribute("title", `AcuGIS ${this.type}`);
    this.iframe.setAttribute("data-acugis-embed-id", this.id);
    this.iframe.setAttribute(
      "sandbox",
      "allow-scripts allow-same-origin allow-forms allow-downloads allow-popups",
    );
    this.componentId = componentId ?? "";
    this.componentPresentation = componentId !== undefined;
    this.contentPresentation = options.presentation === "content";
    // "auto" matches the content-mode convention: the iframe starts at 0
    // and takes the bridge's natural size. An explicit size stays as given.
    this.autoWidth =
      this.componentPresentation &&
      (options.width === undefined || options.width === "auto");
    this.autoHeight = this.componentPresentation
      ? options.height === undefined || options.height === "auto"
      : this.contentPresentation && (options.autoResize === true || options.height === "auto");
    this.iframe.style.border = "0";
    this.iframe.style.display = "block";
    this.iframe.style.width = this.componentPresentation
      ? this.autoWidth
        ? "0px"
        : cssSize(options.width, "0px")
      : cssSize(options.width, "100%");
    this.iframe.style.height = this.componentPresentation
      ? this.autoHeight
        ? "0px"
        : cssSize(options.height, "0px")
      : this.contentPresentation && options.height === "auto"
        ? "0px"
        : cssSize(options.height, "800px");

    const { url } = this.componentPresentation
      ? buildComponentUrl({
          server: this.server,
          report: this.contentId,
          component: this.componentId,
          parameters: this.parameters,
          embedToken: this.auth.token,
        })
      : buildEmbedUrl({
          server: this.server,
          type: this.type,
          id: this.contentId,
          parameters: this.parameters,
          viewer: options.viewer,
          datasource: options.datasource,
          embedToken: this.auth.token,
          autoResize: options.autoResize,
          presentation: options.presentation,
        });

    this.onMessageBound = (event) => this.handleMessage(event);
    window.addEventListener("message", this.onMessageBound);

    this.iframe.addEventListener("load", () => {
      this.iframeWindow = this.iframe.contentWindow;
      // Token is already in the iframe URL (`et=`). Re-send via postMessage only
      // as a secondary channel for viewers that strip query tokens after bootstrap.
      this.post("auth", { type: "token", token: this.auth.token });
      this.post("configure", {
        hostOrigin: this.hostOrigin,
        viewer: options.viewer,
        autoResize: !!options.autoResize,
        parameters: this.parameters,
      });
    });

    this.container.replaceChildren(this.iframe);
    this.iframe.src = url;
  }

  ready(): Promise<void> {
    this.assertAlive();
    return this.readyPromise;
  }

  on<T extends EmbedEventType>(event: T, callback: EmbedEventHandler<T>): this {
    this.assertAlive();
    if (!this.listeners[event]) {
      this.listeners[event] = new Set() as ListenerMap[T];
    }
    (this.listeners[event] as Set<EmbedEventHandler<T>>).add(callback);
    return this;
  }

  off<T extends EmbedEventType>(event: T, callback: EmbedEventHandler<T>): this {
    const set = this.listeners[event] as Set<EmbedEventHandler<T>> | undefined;
    set?.delete(callback);
    return this;
  }

  setParameters(parameters: Parameters): this {
    this.assertAlive();
    this.parameters = { ...this.parameters, ...parameters };
    this.post("setParameters", { parameters: this.parameters });
    return this;
  }

  getParameters(): Parameters {
    return { ...this.parameters };
  }

  refresh(): this {
    this.assertAlive();
    this.post("refresh");
    return this;
  }

  resize(): this {
    this.assertAlive();
    this.post("resize");
    return this;
  }

  setHeight(height: string | number): this {
    this.assertAlive();
    this.iframe.style.height = cssSize(height, this.iframe.style.height || "800px");
    this.post("setHeight", { height: this.iframe.style.height });
    return this;
  }

  setWidth(width: string | number): this {
    this.assertAlive();
    this.iframe.style.width = cssSize(width, this.iframe.style.width || "100%");
    this.post("setWidth", { width: this.iframe.style.width });
    return this;
  }

  nextPage(): this {
    this.assertAlive();
    this.post("nextPage");
    return this;
  }

  previousPage(): this {
    this.assertAlive();
    this.post("previousPage");
    return this;
  }

  goToPage(page: number): this {
    this.assertAlive();
    this.post("goToPage", { page });
    return this;
  }

  zoomIn(): this {
    this.assertAlive();
    this.post("zoomIn");
    return this;
  }

  zoomOut(): this {
    this.assertAlive();
    this.post("zoomOut");
    return this;
  }

  setZoom(percent: number): this {
    this.assertAlive();
    this.post("setZoom", { percent });
    return this;
  }

  export(format: ExportFormat | string): this {
    this.assertAlive();
    if (this.componentPresentation) {
      throw new Error("EXPORT_UNSUPPORTED: component embeds do not support export");
    }
    this.post("export", { format });
    return this;
  }

  destroy(): void {
    if (this.destroyed) {
      return;
    }
    this.destroyed = true;
    window.removeEventListener("message", this.onMessageBound);
    this.iframe.remove();
    for (const key of Object.keys(this.listeners) as EmbedEventType[]) {
      this.listeners[key]?.clear();
    }
    if (!this.readyResolved) {
      this.rejectReady(new Error("DESTROYED: embed destroyed before ready"));
    }
  }

  private assertAlive(): void {
    if (this.destroyed) {
      throw new Error("DESTROYED: embed instance has been destroyed");
    }
  }

  private post(type: string, payload?: unknown): void {
    const win = this.iframeWindow || this.iframe.contentWindow;
    if (!win) {
      return;
    }
    // Never use "*" — always the Report Server origin.
    win.postMessage(createMessage(type, payload), this.serverOrigin);
  }

  private handleMessage(event: MessageEvent): void {
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

  private dispatchProtocol(msg: ProtocolMessage): void {
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
        const payload = (msg.payload || {}) as EmbedErrorEvent;
        const err: EmbedErrorEvent = {
          code: payload.code || "SERVER_ERROR",
          message: payload.message || "Embedded viewer error",
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
        const payload = (msg.payload || {}) as { parameters?: Parameters };
        if (payload.parameters) {
          this.parameters = { ...payload.parameters };
        }
        this.emit("parameterChange", { parameters: this.getParameters() });
        break;
      }
      case "pageChange":
        this.emit("pageChange", (msg.payload || {}) as EmbedEventMap["pageChange"]);
        break;
      case "selection":
        this.emit("selection", (msg.payload || {}) as EmbedEventMap["selection"]);
        break;
      case "filterChange":
        this.emit("filterChange", (msg.payload || {}) as EmbedEventMap["filterChange"]);
        break;
      case "refresh":
        this.emit("refresh", {});
        break;
      case "export":
        this.emit("export", (msg.payload || {}) as EmbedEventMap["export"]);
        break;
      case "resize": {
        const payload = (msg.payload || {}) as { width?: number; height?: number };
        // Viewer embeds keep the existing resize behavior. Content embeds
        // change the iframe height only when the host asked for auto height,
        // so an explicit width and height stay as given. Component embeds
        // apply the bridge's natural document size only when that side is auto.
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
        } else if (
          this.autoHeight &&
          typeof payload.height === "number" &&
          payload.height > 0
        ) {
          this.iframe.style.height = `${payload.height}px`;
        }
        this.emit("resize", {
          width: payload.width || this.iframe.clientWidth,
          height: payload.height || this.iframe.clientHeight,
        });
        break;
      }
      default:
        break;
    }
  }

  private emit<T extends EmbedEventType>(event: T, payload: EmbedEventMap[T]): void {
    const set = this.listeners[event] as Set<EmbedEventHandler<T>> | undefined;
    if (!set) {
      return;
    }
    for (const cb of set) {
      try {
        cb(payload);
      } catch {
        // Host callbacks must not break the SDK.
      }
    }
  }
}
