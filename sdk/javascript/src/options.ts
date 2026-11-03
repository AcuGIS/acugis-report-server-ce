import type { AuthOptions, EmbedOptions, ViewerOptions } from "./types.js";

/** Personal Access Token prefix — never accepted by the Embedding SDK. */
export const PAT_PREFIX = "jrs_";

/** Short-lived embed token prefix issued by POST /api/v1/embed/token. */
export const EMBED_TOKEN_PREFIX = "emb_";

export function normalizeServer(server: string): string {
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

export function resolveTarget(target: string | HTMLElement): HTMLElement {
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

export function isEmbedToken(token: string): boolean {
  return token.startsWith(EMBED_TOKEN_PREFIX);
}

export function isPersonalAccessToken(token: string): boolean {
  return token.startsWith(PAT_PREFIX);
}

/**
 * Resolve and validate embed auth.
 * Embed tokens are mandatory — session auth and PATs are rejected.
 */
export function normalizeAuth(options: EmbedOptions): AuthOptions {
  let token = "";
  if (options.auth) {
    if (options.auth.type !== "token") {
      throw new Error(
        "AUTH_REQUIRED: embedding requires an embed token (auth.type must be \"token\")",
      );
    }
    token = (options.auth.token || "").trim();
  } else if (options.token) {
    token = options.token.trim();
  }

  if (!token) {
    throw new Error(
      "AUTH_REQUIRED: embedding requires a short-lived embed token (emb_…). " +
        "Mint one via POST /api/v1/embed/token from your backend.",
    );
  }

  if (isPersonalAccessToken(token)) {
    throw new Error(
      "AUTH_REQUIRED: personal access tokens (jrs_…) cannot be used in the browser SDK. " +
        "Exchange the PAT server-side for an embed token via POST /api/v1/embed/token.",
    );
  }

  if (!isEmbedToken(token)) {
    throw new Error(
      "TOKEN_INVALID: expected an embed token starting with emb_. " +
        "Mint one via POST /api/v1/embed/token.",
    );
  }

  return { type: "token", token };
}

export function cssSize(value: string | number | undefined, fallback: string): string {
  if (value === undefined || value === null || value === "") {
    return fallback;
  }
  if (typeof value === "number") {
    return `${value}px`;
  }
  return String(value);
}

export function serializeParameters(
  parameters: Record<string, unknown> | undefined,
): Record<string, string> {
  const out: Record<string, string> = {};
  if (!parameters) {
    return out;
  }
  for (const [key, value] of Object.entries(parameters)) {
    if (value === undefined || value === null) {
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

/**
 * Designer Embed ID. Values are matched as given: not trimmed into another
 * id and not lowercased.
 */
export const COMPONENT_ID_PATTERN = /^[a-z][a-z0-9_-]{0,63}$/;

export function assertComponentId(component: string): string {
  if (typeof component !== "string" || !COMPONENT_ID_PATTERN.test(component)) {
    throw new Error(
      "INVALID_PARAMETER: component must match ^[a-z][a-z0-9_-]{0,63}$",
    );
  }
  return component;
}

export function defaultViewerOptions(viewer?: ViewerOptions): Required<ViewerOptions> {
  return {
    toolbar: viewer?.toolbar !== false,
    parameters: viewer?.parameters !== false,
    export: viewer?.export !== false,
    pagination: viewer?.pagination !== false,
    zoom: viewer?.zoom !== false,
    close: viewer?.close === true,
  };
}
