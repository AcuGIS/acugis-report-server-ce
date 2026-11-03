import { afterEach, describe, expect, it, vi } from "vitest";
import { AcuGIS, Embed } from "../src/index.js";
import {
  createMessage,
  isProtocolMessage,
  validateIncomingMessage,
} from "../src/protocol.js";
import { buildComponentUrl, buildEmbedUrl, resolveContentId, resolveContentType } from "../src/url.js";
import {
  normalizeAuth,
  resolveTarget,
  serializeParameters,
} from "../src/options.js";
import { PROTOCOL_SOURCE, PROTOCOL_VERSION } from "../src/types.js";

const EMB = "emb_testpayload.signature";

afterEach(() => {
  document.body.innerHTML = "";
});

describe("target resolution", () => {
  it("resolves CSS selectors", () => {
    const el = document.createElement("div");
    el.id = "report";
    document.body.appendChild(el);
    expect(resolveTarget("#report")).toBe(el);
  });

  it("accepts HTMLElement", () => {
    const el = document.createElement("div");
    expect(resolveTarget(el)).toBe(el);
  });

  it("throws for missing selector", () => {
    expect(() => resolveTarget("#missing")).toThrow(/INVALID_TARGET/);
  });
});

describe("URL generation", () => {
  it("requires embed token in URL", () => {
    const { url, origin } = buildEmbedUrl({
      server: "https://reports.example.com",
      type: "report",
      id: "Invoice",
      embedToken: EMB,
      parameters: { p_country: "USA", p_year: 2026 },
    });
    expect(origin).toBe("https://reports.example.com");
    expect(url).toContain("/embed/report/Invoice");
    expect(url).toContain("embed=1");
    expect(url).toContain("et=emb_");
    expect(url).toContain("p_country=USA");
  });

  it("builds dashboard/view/map-report paths", () => {
    expect(
      buildEmbedUrl({
        server: "https://reports.example.com/go",
        type: "dashboard",
        id: "alice/sales",
        embedToken: EMB,
      }).url,
    ).toContain("/go/embed/dashboard/alice/sales");

    expect(
      buildEmbedUrl({
        server: "https://reports.example.com",
        type: "view",
        id: "bob/q1",
        embedToken: EMB,
      }).url,
    ).toContain("/embed/view/bob/q1");

    expect(
      buildEmbedUrl({
        server: "https://reports.example.com",
        type: "map-report",
        id: "parcels",
        embedToken: EMB,
      }).url,
    ).toContain("/embed/map-report/parcels");
  });

  it("rejects missing embed token", () => {
    expect(() =>
      buildEmbedUrl({
        server: "https://reports.example.com",
        type: "report",
        id: "Invoice",
      }),
    ).toThrow(/AUTH_REQUIRED/);
  });

  it("rejects PAT in embed URL", () => {
    expect(() =>
      buildEmbedUrl({
        server: "https://reports.example.com",
        type: "report",
        id: "Invoice",
        embedToken: "jrs_secretpat",
      }),
    ).toThrow(/personal access tokens/);
  });

  it("encodes viewer chrome flags", () => {
    const { url } = buildEmbedUrl({
      server: "https://reports.example.com",
      type: "report",
      id: "Invoice",
      embedToken: EMB,
      viewer: { toolbar: false, parameters: false, export: false },
    });
    expect(url).toContain("toolbar=0");
    expect(url).toContain("parameters=0");
    expect(url).toContain("export=0");
  });
});

describe("content id/type", () => {
  it("resolves typed convenience fields", () => {
    expect(resolveContentType({ server: "https://x", target: "#a", reportId: "r" })).toBe(
      "report",
    );
    expect(
      resolveContentId(
        { server: "https://x", target: "#a", dashboardId: "o/k" },
        "dashboard",
      ),
    ).toBe("o/k");
  });
});

describe("parameters", () => {
  it("serializes multi-value parameters", () => {
    expect(
      serializeParameters({
        p_state: ["CA", "TX", "NY"],
        p_year: 2026,
      }),
    ).toEqual({
      p_state: "CA,TX,NY",
      p_year: "2026",
    });
  });
});

describe("auth — embed tokens required", () => {
  it("requires emb_ token", () => {
    expect(() => normalizeAuth({ server: "https://x", target: "#a" })).toThrow(
      /AUTH_REQUIRED/,
    );
  });

  it("rejects session auth", () => {
    expect(() =>
      normalizeAuth({
        server: "https://x",
        target: "#a",
        auth: { type: "session" } as never,
      }),
    ).toThrow(/AUTH_REQUIRED/);
  });

  it("rejects personal access tokens", () => {
    expect(() =>
      normalizeAuth({
        server: "https://x",
        target: "#a",
        token: "jrs_abc123",
      }),
    ).toThrow(/personal access tokens/);
  });

  it("accepts emb_ token convenience field", () => {
    expect(
      normalizeAuth({
        server: "https://x",
        target: "#a",
        token: EMB,
      }),
    ).toEqual({ type: "token", token: EMB });
  });

  it("rejects non-emb tokens", () => {
    expect(() =>
      normalizeAuth({
        server: "https://x",
        target: "#a",
        token: "random-token",
      }),
    ).toThrow(/TOKEN_INVALID/);
  });
});

describe("postMessage validation", () => {
  it("accepts well-formed protocol messages", () => {
    expect(isProtocolMessage(createMessage("load"))).toBe(true);
  });

  it("rejects malformed payloads", () => {
    expect(isProtocolMessage({ source: "other", version: 1, type: "load" })).toBe(false);
    expect(isProtocolMessage({ source: PROTOCOL_SOURCE, version: 99, type: "load" })).toBe(
      false,
    );
  });

  it("rejects unauthorized origins", () => {
    const iframe = {} as Window;
    const event = {
      origin: "https://evil.example.com",
      source: iframe,
      data: createMessage("load"),
    } as MessageEvent;
    expect(
      validateIncomingMessage(event, "https://reports.example.com", iframe),
    ).toBeNull();
  });

  it("rejects wrong source window", () => {
    const iframe = {} as Window;
    const other = {} as Window;
    const event = {
      origin: "https://reports.example.com",
      source: other,
      data: createMessage("load"),
    } as MessageEvent;
    expect(
      validateIncomingMessage(event, "https://reports.example.com", iframe),
    ).toBeNull();
  });

  it("accepts matching origin and source", () => {
    const iframe = {} as Window;
    const event = {
      origin: "https://reports.example.com",
      source: iframe,
      data: {
        source: PROTOCOL_SOURCE,
        version: PROTOCOL_VERSION,
        type: "load",
      },
    } as MessageEvent;
    expect(
      validateIncomingMessage(event, "https://reports.example.com", iframe)?.type,
    ).toBe("load");
  });
});

describe("iframe embed instances", () => {
  it("creates an iframe with et= embed token", () => {
    const el = document.createElement("div");
    el.id = "report";
    document.body.appendChild(el);
    const report = AcuGIS.embedReport({
      server: "https://reports.example.com",
      target: "#report",
      reportId: "Invoice",
      token: EMB,
    });
    const iframe = el.querySelector("iframe");
    expect(iframe).toBeTruthy();
    expect(iframe?.src).toContain("/embed/report/Invoice");
    expect(iframe?.src).toContain("et=emb_");
    report.destroy();
  });

  it("supports multiple embeds without shared state", () => {
    const a = document.createElement("div");
    const b = document.createElement("div");
    document.body.append(a, b);
    const r1 = AcuGIS.embedReport({
      server: "https://reports.example.com",
      target: a,
      reportId: "A",
      token: EMB,
      parameters: { p: 1 },
    });
    const r2 = AcuGIS.embedReport({
      server: "https://other.example.com",
      target: b,
      reportId: "B",
      token: "emb_other.sig",
      parameters: { p: 2 },
    });
    expect(r1.id).not.toBe(r2.id);
    expect(r1.getParameters()).toEqual({ p: 1 });
    expect(r2.getParameters()).toEqual({ p: 2 });
    r1.setParameters({ p: 10 });
    expect(r1.getParameters().p).toBe(10);
    expect(r2.getParameters().p).toBe(2);
    r1.destroy();
    r2.destroy();
  });

  it("destroy removes iframe and blocks further calls", () => {
    const el = document.createElement("div");
    document.body.appendChild(el);
    const report = AcuGIS.embedReport({
      server: "https://reports.example.com",
      target: el,
      reportId: "Invoice",
      token: EMB,
    });
    report.destroy();
    expect(el.querySelector("iframe")).toBeNull();
    expect(() => report.refresh()).toThrow(/DESTROYED/);
  });

  it("emits ACCESS_DENIED and TOKEN_EXPIRED", () => {
    const el = document.createElement("div");
    document.body.appendChild(el);
    const report = new Embed({
      server: "https://reports.example.com",
      target: el,
      reportId: "missing",
      token: EMB,
    });
    const codes: string[] = [];
    report.on("error", (e) => codes.push(e.code));
    report["iframeWindow"] = {} as Window;
    for (const code of ["ACCESS_DENIED", "TOKEN_EXPIRED", "CONTENT_NOT_FOUND"]) {
      window.dispatchEvent(
        new MessageEvent("message", {
          origin: "https://reports.example.com",
          source: report["iframeWindow"] as MessageEventSource,
          data: createMessage("error", { code, message: code }),
        }),
      );
    }
    expect(codes).toEqual(["ACCESS_DENIED", "TOKEN_EXPIRED", "CONTENT_NOT_FOUND"]);
    report.destroy();
  });

  it("updates height on autoResize messages", () => {
    const el = document.createElement("div");
    document.body.appendChild(el);
    const report = AcuGIS.embedReport({
      server: "https://reports.example.com",
      target: el,
      reportId: "Invoice",
      token: EMB,
      autoResize: true,
      height: "400px",
    });
    report["iframeWindow"] = {} as Window;
    window.dispatchEvent(
      new MessageEvent("message", {
        origin: "https://reports.example.com",
        source: report["iframeWindow"] as MessageEventSource,
        data: createMessage("resize", { width: 800, height: 1200 }),
      }),
    );
    expect(report["iframe"].style.height).toBe("1200px");
    report.destroy();
  });

  it("keeps the viewer URL when presentation is omitted", () => {
    const el = document.createElement("div");
    document.body.appendChild(el);
    const report = AcuGIS.embedReport({
      server: "https://reports.example.com",
      target: el,
      reportId: "Invoice",
      token: EMB,
      parameters: { year: 2026 },
    });
    const iframe = el.querySelector("iframe");
    expect(iframe?.src).toContain("/embed/report/Invoice");
    expect(iframe?.src).not.toContain("/embed/report/Invoice/content");
    expect(iframe?.src).toContain("embed=1");
    expect(iframe?.src).toContain("year=2026");
    expect(el.querySelectorAll("iframe")).toHaveLength(1);
    report.destroy();
  });

  it("builds a chrome-less content URL from presentation content", () => {
    const { url } = buildEmbedUrl({
      server: "https://reports.example.com",
      type: "report",
      id: "portfolioperformance",
      presentation: "content",
      embedToken: EMB,
      parameters: { year: 2026, _repName: "other", _repFormat: "pdf" },
      viewer: { toolbar: false, parameters: false, export: false, zoom: false },
    });
    expect(url).toBe(
      "https://reports.example.com/embed/report/portfolioperformance/content?et=emb_testpayload.signature&year=2026",
    );

    const el = document.createElement("div");
    document.body.appendChild(el);
    const embedded = AcuGIS.embedReport({
      server: "https://reports.example.com",
      target: el,
      report: "portfolioperformance",
      presentation: "content",
      auth: { type: "token", token: EMB },
      parameters: { year: 2026 },
      viewer: { toolbar: false, parameters: false, export: false, zoom: false },
      width: "100%",
      height: 720,
    });
    const frames = el.querySelectorAll("iframe");
    expect(frames).toHaveLength(1);
    const src = frames[0]?.src || "";
    expect(src).toContain("/embed/report/portfolioperformance/content");
    expect(src).toContain("et=emb_");
    expect(src).toContain("year=2026");
    expect(src).not.toContain("embed=1");
    expect(src).not.toContain("toolbar");
    expect(frames[0]?.style.width).toBe("100%");
    expect(frames[0]?.style.height).toBe("720px");
    embedded.destroy();
  });

  it("rejects jrs_ tokens for content presentation", () => {
    const el = document.createElement("div");
    document.body.appendChild(el);
    expect(() =>
      AcuGIS.embedReport({
        server: "https://reports.example.com",
        target: el,
        report: "portfolioperformance",
        presentation: "content",
        token: "jrs_secretpat",
      }),
    ).toThrow(/personal access tokens/);
  });

  it("uses content height when height is auto and keeps an explicit height", () => {
    const autoHost = document.createElement("div");
    const fixedHost = document.createElement("div");
    document.body.append(autoHost, fixedHost);
    const auto = AcuGIS.embedReport({
      server: "https://reports.example.com",
      target: autoHost,
      report: "portfolioperformance",
      presentation: "content",
      token: EMB,
      width: "100%",
      height: "auto",
    });
    const fixed = AcuGIS.embedReport({
      server: "https://reports.example.com",
      target: fixedHost,
      report: "portfolioperformance",
      presentation: "content",
      token: EMB,
      width: "100%",
      height: 720,
    });
    expect(auto["iframe"].style.height).toBe("0px");
    expect(auto["iframe"].style.width).toBe("100%");
    auto["iframeWindow"] = {} as Window;
    fixed["iframeWindow"] = {} as Window;
    for (const instance of [auto, fixed]) {
      window.dispatchEvent(
        new MessageEvent("message", {
          origin: "https://reports.example.com",
          source: instance["iframeWindow"] as MessageEventSource,
          data: createMessage("resize", { width: 640, height: 900 }),
        }),
      );
    }
    expect(auto["iframe"].style.height).toBe("900px");
    expect(auto["iframe"].style.width).toBe("100%");
    expect(fixed["iframe"].style.height).toBe("720px");
    expect(fixed["iframe"].style.width).toBe("100%");
    auto.destroy();
    fixed.destroy();
  });

  it("does not change dashboard, view, or map report URLs", () => {
    const dash = buildEmbedUrl({
      server: "https://reports.example.com",
      type: "dashboard",
      id: "alice/sales",
      embedToken: EMB,
      presentation: "content",
    });
    expect(dash.url).toContain("/embed/dashboard/alice/sales");
    expect(dash.url).not.toContain("/content");
    expect(dash.url).toContain("embed=1");
  });

  it("rejects embed without token at construction", () => {
    const el = document.createElement("div");
    document.body.appendChild(el);
    expect(() =>
      AcuGIS.embedReport({
        server: "https://reports.example.com",
        target: el,
        reportId: "Invoice",
      }),
    ).toThrow(/AUTH_REQUIRED/);
  });
});

const SERVER = "https://reports.example.com";
const ORIGIN = "https://reports.example.com";

function host(id = "host"): HTMLElement {
  const el = document.createElement("div");
  el.id = id;
  document.body.appendChild(el);
  return el;
}

function embedComponent(
  target: HTMLElement,
  overrides: Partial<{
    server: string;
    report: string;
    component: string;
    auth: { type: "token"; token: string };
    parameters: Record<string, string | number>;
    width: string | number;
    height: string | number;
  }> = {},
) {
  return AcuGIS.embedComponent({
    server: SERVER,
    report: "portfolioperformance",
    component: "monthly-occupancy",
    target,
    auth: { type: "token", token: EMB },
    ...overrides,
  });
}

function frameOf(target: HTMLElement): HTMLIFrameElement {
  const iframe = target.querySelector("iframe");
  if (!(iframe instanceof HTMLIFrameElement)) {
    throw new Error("missing iframe");
  }
  return iframe;
}

function post(
  component: Embed,
  type: string,
  payload: unknown,
  origin = ORIGIN,
  source?: Window,
) {
  const win = source ?? (component["iframeWindow"] as Window);
  window.dispatchEvent(
    new MessageEvent("message", {
      origin,
      source: win as MessageEventSource,
      data: createMessage(type, payload),
    }),
  );
}

describe("embedComponent", () => {
  it("builds the component route", () => {
    const el = host();
    const component = embedComponent(el);
    const iframe = frameOf(el);
    const url = new URL(iframe.src);
    expect(url.origin).toBe(ORIGIN);
    expect(url.pathname).toBe(
      "/embed/report/portfolioperformance/component/monthly-occupancy",
    );
    expect(url.pathname).not.toContain("/content");
    expect(url.searchParams.get("embed")).toBeNull();
    expect(url.searchParams.has("toolbar")).toBe(false);
    expect(el.querySelectorAll("iframe")).toHaveLength(1);
    expect(iframe.getAttribute("sandbox")).toBe(
      "allow-scripts allow-same-origin allow-forms allow-downloads allow-popups",
    );
    expect(iframe.style.width).toBe("0px");
    expect(iframe.style.height).toBe("0px");
    component.destroy();
  });

  it("builds a map embed id on the ordinary component route", () => {
    const el = host();
    const component = embedComponent(el, {
      component: "property-map",
      parameters: { year: 2026 },
      width: "auto",
      height: "auto",
    });
    const url = new URL(frameOf(el).src);
    expect(url.pathname).toBe("/embed/report/portfolioperformance/component/property-map");
    expect(url.searchParams.get("year")).toBe("2026");
    expect(url.pathname).not.toContain("/map/");
    expect(url.searchParams.get("account_id")).toBeNull();
    expect(frameOf(el).style.width).toBe("0px");
    expect(frameOf(el).style.height).toBe("0px");
    expect("embedMap" in AcuGIS).toBe(false);
    component.destroy();
  });

  it("sends the emb_ token", () => {
    const el = host();
    const component = embedComponent(el);
    const url = new URL(frameOf(el).src);
    expect(url.searchParams.get("et")).toBe(EMB);
    expect(url.toString()).toContain("et=emb_");
    expect(url.toString()).not.toContain("jrs_");
    component.destroy();
  });

  it("rejects a jrs_ personal access token", () => {
    const el = host();
    expect(() =>
      embedComponent(el, { auth: { type: "token", token: "jrs_secretpat" } }),
    ).toThrow(/personal access tokens/);
    expect(el.querySelector("iframe")).toBeNull();
  });

  it("validates the component id without rewriting it", () => {
    for (const id of ["monthly-occupancy", "total-units", "total_units", "chart1", "a"]) {
      const el = host(id);
      const component = embedComponent(el, { component: id });
      expect(new URL(frameOf(el).src).pathname).toBe(
        `/embed/report/portfolioperformance/component/${id}`,
      );
      component.destroy();
    }
    for (const id of ["Monthly Occupancy", "1chart", "chart!", "chartA", " monthly-occupancy", "monthly-occupancy "]) {
      const el = host(`bad-${id.length}`);
      expect(() => embedComponent(el, { component: id })).toThrow(/INVALID_PARAMETER/);
      expect(el.querySelector("iframe")).toBeNull();
    }
  });

  it("serializes parameters the same way as embedReport", () => {
    const parameters = { year: 2026, property_id: "" };
    const content = new URL(
      buildEmbedUrl({
        server: SERVER,
        type: "report",
        id: "portfolioperformance",
        presentation: "content",
        embedToken: EMB,
        parameters,
      }).url,
    );
    const componentURL = new URL(
      buildComponentUrl({
        server: SERVER,
        report: "portfolioperformance",
        component: "monthly-occupancy",
        embedToken: EMB,
        parameters: {
          ...parameters,
          account_id: "smith-residential",
          _componentId: "total-units",
          _repName: "other",
          _dataSource: "evil",
          datasource: "evil",
        },
      }).url,
    );
    expect(componentURL.pathname).toBe(
      "/embed/report/portfolioperformance/component/monthly-occupancy",
    );
    expect(componentURL.searchParams.get("et")).toBe(content.searchParams.get("et"));
    expect(componentURL.searchParams.get("year")).toBe("2026");
    expect(componentURL.searchParams.get("year")).toBe(content.searchParams.get("year"));
    expect(componentURL.searchParams.get("property_id")).toBe("");
    expect(componentURL.searchParams.get("property_id")).toBe(
      content.searchParams.get("property_id"),
    );
    expect(componentURL.searchParams.get("account_id")).toBe("smith-residential");
    expect(componentURL.searchParams.get("_componentId")).toBeNull();
    expect(componentURL.searchParams.get("_repName")).toBeNull();
    expect(componentURL.searchParams.get("_dataSource")).toBeNull();
    expect(componentURL.searchParams.get("datasource")).toBeNull();
    expect(componentURL.searchParams.get("embed")).toBeNull();

    const el = host();
    const component = embedComponent(el, { parameters });
    const live = new URL(frameOf(el).src);
    expect(live.pathname).toBe(componentURL.pathname);
    expect(live.searchParams.get("year")).toBe("2026");
    expect(live.searchParams.get("property_id")).toBe("");
    expect(live.searchParams.get("et")).toBe(EMB);
    component.destroy();
  });

  it("sets iframe width from a trusted natural width when width is auto", () => {
    const el = host();
    const component = embedComponent(el, { width: "auto", height: "auto" });
    component["iframeWindow"] = {} as Window;
    post(component, "resize", { width: 320, height: 180 });
    expect(frameOf(el).style.width).toBe("320px");
    component.destroy();
  });

  it("sets iframe height from a trusted natural height when height is auto", () => {
    const el = host();
    const component = embedComponent(el, { width: "auto", height: "auto" });
    component["iframeWindow"] = {} as Window;
    post(component, "resize", { width: 320, height: 180 });
    expect(frameOf(el).style.height).toBe("180px");
    component.destroy();
  });

  it("keeps an explicit width", () => {
    const el = host();
    const component = embedComponent(el, { width: "100%", height: "auto" });
    expect(frameOf(el).style.width).toBe("100%");
    component["iframeWindow"] = {} as Window;
    post(component, "resize", { width: 320, height: 180 });
    expect(frameOf(el).style.width).toBe("100%");
    expect(frameOf(el).style.height).toBe("180px");
    component.destroy();
  });

  it("keeps an explicit height", () => {
    const el = host();
    const component = embedComponent(el, { width: "auto", height: "400px" });
    expect(frameOf(el).style.height).toBe("400px");
    component["iframeWindow"] = {} as Window;
    post(component, "resize", { width: 320, height: 180 });
    expect(frameOf(el).style.height).toBe("400px");
    expect(frameOf(el).style.width).toBe("320px");
    component.destroy();
  });

  it("applies a rendered 80 by 24 field only when that side is auto", () => {
    const autoHost = host("units-auto");
    const auto = embedComponent(autoHost, {
      component: "total-units",
      width: "auto",
      height: "auto",
    });
    auto["iframeWindow"] = {} as Window;
    post(auto, "resize", { width: 96, height: 40 });
    expect(frameOf(autoHost).style.width).toBe("96px");
    expect(frameOf(autoHost).style.height).toBe("40px");

    const fixedHost = host("units-fixed");
    const fixed = embedComponent(fixedHost, {
      component: "total-units",
      width: 80,
      height: 24,
    });
    expect(frameOf(fixedHost).style.width).toBe("80px");
    expect(frameOf(fixedHost).style.height).toBe("24px");
    fixed["iframeWindow"] = {} as Window;
    post(fixed, "resize", { width: 96, height: 40 });
    expect(frameOf(fixedHost).style.width).toBe("80px");
    expect(frameOf(fixedHost).style.height).toBe("24px");
    auto.destroy();
    fixed.destroy();
  });

  it("applies a rendered 544 by 96 chart only when that side is auto", () => {
    const autoHost = host("chart-auto");
    const auto = embedComponent(autoHost, {
      component: "monthly-occupancy",
    });
    expect(frameOf(autoHost).style.width).toBe("0px");
    expect(frameOf(autoHost).style.height).toBe("0px");
    auto["iframeWindow"] = {} as Window;
    post(auto, "resize", { width: 560, height: 116 });
    expect(frameOf(autoHost).style.width).toBe("560px");
    expect(frameOf(autoHost).style.height).toBe("116px");

    const fixedHost = host("chart-fixed");
    const fixed = embedComponent(fixedHost, {
      component: "monthly-occupancy",
      width: 544,
      height: 96,
    });
    fixed["iframeWindow"] = {} as Window;
    post(fixed, "resize", { width: 560, height: 116 });
    expect(frameOf(fixedHost).style.width).toBe("544px");
    expect(frameOf(fixedHost).style.height).toBe("96px");
    auto.destroy();
    fixed.destroy();
  });

  it("ignores resize, error, and load messages from another origin", () => {
    const el = host();
    const component = embedComponent(el);
    const codes: string[] = [];
    let loaded = false;
    component.on("error", (event) => codes.push(event.code));
    component.on("load", () => {
      loaded = true;
    });
    component["iframeWindow"] = {} as Window;
    post(component, "resize", { width: 320, height: 180 }, "https://evil.example.com");
    post(
      component,
      "error",
      { code: "COMPONENT_NOT_FOUND", message: "missing" },
      "https://evil.example.com",
    );
    post(component, "load", {}, "https://evil.example.com");
    expect(frameOf(el).style.width).toBe("0px");
    expect(frameOf(el).style.height).toBe("0px");
    expect(codes).toEqual([]);
    expect(loaded).toBe(false);
    component.destroy();
  });

  it("ignores messages from another window", () => {
    const el = host();
    const component = embedComponent(el);
    const own = {} as Window;
    const other = {} as Window;
    component["iframeWindow"] = own;
    post(component, "resize", { width: 320, height: 180 }, ORIGIN, other);
    expect(frameOf(el).style.width).toBe("0px");
    expect(frameOf(el).style.height).toBe("0px");
    post(component, "resize", { width: 320, height: 180 }, ORIGIN, own);
    expect(frameOf(el).style.width).toBe("320px");
    expect(frameOf(el).style.height).toBe("180px");
    component.destroy();
  });

  it("resizes two components independently", () => {
    const units = host("units");
    const occupancy = host("occupancy");
    const total = embedComponent(units, { component: "total-units" });
    const chart = embedComponent(occupancy, { component: "monthly-occupancy" });
    expect(new URL(frameOf(units).src).pathname).toBe(
      "/embed/report/portfolioperformance/component/total-units",
    );
    expect(new URL(frameOf(occupancy).src).pathname).toBe(
      "/embed/report/portfolioperformance/component/monthly-occupancy",
    );
    expect(frameOf(units)).not.toBe(frameOf(occupancy));
    total["iframeWindow"] = {} as Window;
    chart["iframeWindow"] = {} as Window;
    post(total, "resize", { width: 220, height: 28 });
    post(chart, "resize", { width: 320, height: 180 });
    expect(frameOf(units).style.width).toBe("220px");
    expect(frameOf(units).style.height).toBe("28px");
    expect(frameOf(occupancy).style.width).toBe("320px");
    expect(frameOf(occupancy).style.height).toBe("180px");
    post(total, "resize", { width: 180, height: 20 });
    expect(frameOf(units).style.width).toBe("180px");
    expect(frameOf(occupancy).style.width).toBe("320px");
    expect(frameOf(occupancy).style.height).toBe("180px");
    total.destroy();
    chart.destroy();
  });

  it("destroy removes the iframe and ignores later messages", () => {
    const units = host("units");
    const occupancy = host("occupancy");
    const total = embedComponent(units, { component: "total-units" });
    const chart = embedComponent(occupancy, { component: "monthly-occupancy" });
    const detached = frameOf(units);
    const totalWindow = {} as Window;
    const chartWindow = {} as Window;
    total["iframeWindow"] = totalWindow;
    chart["iframeWindow"] = chartWindow;
    total.destroy();
    expect(units.querySelector("iframe")).toBeNull();
    expect(detached.isConnected).toBe(false);
    post(total, "resize", { width: 999, height: 999 }, ORIGIN, totalWindow);
    post(
      total,
      "error",
      { code: "COMPONENT_NOT_FOUND", message: "late" },
      ORIGIN,
      totalWindow,
    );
    expect(detached.style.width).toBe("0px");
    expect(detached.style.height).toBe("0px");
    expect(occupancy.querySelector("iframe")).toBeTruthy();
    expect(() => total.export("pdf")).toThrow(/DESTROYED/);
    post(chart, "resize", { width: 320, height: 180 }, ORIGIN, chartWindow);
    expect(frameOf(occupancy).style.width).toBe("320px");
    expect(frameOf(occupancy).style.height).toBe("180px");
    chart.destroy();
  });

  it("surfaces COMPONENT_NOT_FOUND through the error lifecycle", async () => {
    const el = host();
    const component = embedComponent(el);
    const errors: Array<{ code: string; message: string }> = [];
    component.on("error", (event) => errors.push(event));
    const pending = component.ready();
    component["iframeWindow"] = {} as Window;
    post(component, "error", {
      code: "COMPONENT_NOT_FOUND",
      message: "no design element with this key",
    });
    expect(errors).toEqual([
      {
        code: "COMPONENT_NOT_FOUND",
        message: "no design element with this key",
      },
    ]);
    await expect(pending).rejects.toThrow(
      "COMPONENT_NOT_FOUND: no design element with this key",
    );
    component.destroy();
  });

  it("rejects component export", () => {
    const el = host();
    const component = embedComponent(el);
    const src = frameOf(el).src;
    expect(() => component.export("pdf")).toThrow(/EXPORT_UNSUPPORTED/);
    expect(frameOf(el).src).toBe(src);
    expect(new URL(src).pathname).toContain("/component/");
    expect(src).not.toContain("/content");

    const reportHost = host("report");
    const report = AcuGIS.embedReport({
      server: SERVER,
      target: reportHost,
      report: "portfolioperformance",
      token: EMB,
    });
    expect(() => report.export("pdf")).not.toThrow();
    component.destroy();
    report.destroy();
  });
});
