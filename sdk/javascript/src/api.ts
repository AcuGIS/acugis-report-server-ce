import { Embed } from "./embed.js";
import type { EmbedComponentOptions, EmbedOptions } from "./types.js";

function withType(options: EmbedOptions, type: EmbedOptions["type"], idField: keyof EmbedOptions): EmbedOptions {
  const id = (options[idField] as string | undefined) || options.id;
  return {
    ...options,
    type,
    id,
  };
}

/** Auto-initialize declarative embeds: <div class="acugis-report" data-report-id="…"> */
export function autoInit(root: ParentNode = document): Embed[] {
  const nodes = root.querySelectorAll<HTMLElement>(
    ".acugis-report, .acugis-dashboard, .acugis-view, .acugis-map-report",
  );
  const embeds: Embed[] = [];
  nodes.forEach((el) => {
    if (el.getAttribute("data-acugis-initialized") === "1") {
      return;
    }
    const server = el.getAttribute("data-server") || "";
    const token = el.getAttribute("data-token") || "";
    if (!server || !token) {
      return;
    }
    let options: EmbedOptions = { server, target: el, token };
    if (el.classList.contains("acugis-dashboard") || el.dataset.dashboardId) {
      options = {
        ...options,
        type: "dashboard",
        dashboardId: el.dataset.dashboardId || el.getAttribute("data-dashboard-id") || undefined,
      };
    } else if (el.classList.contains("acugis-view") || el.dataset.viewId) {
      options = {
        ...options,
        type: "view",
        viewId: el.dataset.viewId || el.getAttribute("data-view-id") || undefined,
      };
    } else if (el.classList.contains("acugis-map-report") || el.dataset.mapReportId) {
      options = {
        ...options,
        type: "map-report",
        mapReportId:
          el.dataset.mapReportId || el.getAttribute("data-map-report-id") || undefined,
      };
    } else {
      options = {
        ...options,
        type: "report",
        reportId: el.dataset.reportId || el.getAttribute("data-report-id") || undefined,
      };
    }
    try {
      const embed = new Embed(options);
      el.setAttribute("data-acugis-initialized", "1");
      embeds.push(embed);
    } catch {
      // Ignore invalid declarative nodes.
    }
  });
  return embeds;
}

export const AcuGIS = {
  embed(options: EmbedOptions): Embed {
    return new Embed(options);
  },
  embedReport(options: EmbedOptions): Embed {
    const reportId = options.reportId || options.report;
    return new Embed(withType({ ...options, reportId }, "report", "reportId"));
  },
  embedComponent(options: EmbedComponentOptions): Embed {
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
        origin: options.origin,
      },
      options.component,
    );
  },
  embedDashboard(options: EmbedOptions): Embed {
    return new Embed(withType(options, "dashboard", "dashboardId"));
  },
  embedView(options: EmbedOptions): Embed {
    return new Embed(withType(options, "view", "viewId"));
  },
  embedMapReport(options: EmbedOptions): Embed {
    return new Embed(withType(options, "map-report", "mapReportId"));
  },
  autoInit,
};

export type AcuGISAPI = typeof AcuGIS;
