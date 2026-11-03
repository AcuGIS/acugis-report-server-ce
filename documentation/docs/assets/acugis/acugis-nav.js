/**
 * AcuGIS shared global navigation
 * Renders from a single product catalog into [data-acugis-nav].
 *
 * Docs copy of main-website/shared/acugis-nav.js.
 * Product images resolve beside this script (products/) so nested
 * MkDocs pages can load them. Markup, menus, and links match the website.
 */
(function () {
  "use strict";

  var PRODUCT_ASSET_BASE = (function () {
    var script = document.currentScript;
    if (script && script.src) {
      return new URL("products/", script.src).href;
    }
    return "/shared/products/";
  })();

  var ICONS = {
    "report-server":
      '<svg viewBox="0 0 40 40" width="40" height="40" aria-hidden="true"><rect width="40" height="40" rx="8" fill="#5B4CDB"/><rect x="9" y="22" width="6" height="9" rx="1.2" fill="#fff"/><rect x="17" y="16" width="6" height="15" rx="1.2" fill="#fff"/><rect x="25" y="10" width="6" height="21" rx="1.2" fill="#C4B5FD"/></svg>',
    "report-designer":
      '<svg viewBox="0 0 40 40" width="40" height="40" aria-hidden="true"><rect width="40" height="40" rx="8" fill="#5B4CDB"/><rect x="11" y="10" width="14" height="20" rx="1.6" fill="#fff"/><path d="M14 16h8M14 20h8M14 24h5" stroke="#5B4CDB" stroke-width="1.5" stroke-linecap="round"/><path d="M24.5 22.5l6.2-6.2 2.3 2.3-6.2 6.2-2.8.5z" fill="#C4B5FD"/></svg>',
    "qgis-publisher":
      '<svg viewBox="0 0 40 40" width="40" height="40" aria-hidden="true"><rect width="40" height="40" rx="8" fill="#0D652D"/><path d="M11.5 14.2l8.5-4.2 8.5 4.2v13.2L20 31.4l-8.5-4z" fill="none" stroke="#fff" stroke-width="1.8" stroke-linejoin="round"/><path d="M20 10v21.4M11.5 14.2L20 18.2l8.5-4" fill="none" stroke="#fff" stroke-width="1.8" stroke-linejoin="round"/><circle cx="16.2" cy="20.4" r="1.35" fill="#CEFAD6"/><circle cx="24.4" cy="23.2" r="1.35" fill="#CEFAD6"/><path d="M16.2 20.4c2.2 1.4 5.4 2.2 8.2 2.8" fill="none" stroke="#CEFAD6" stroke-width="1.3" stroke-linecap="round"/></svg>',
    drive:
      '<svg viewBox="0 0 28 28" width="28" height="28" aria-hidden="true"><rect width="28" height="28" rx="6" fill="#0D9488"/><path d="M8 12.2h12l-1.4 7.2H9.4z" fill="#fff"/><path d="M9.2 12.2l1.3-3.4h7l1.3 3.4" fill="none" stroke="#fff" stroke-width="1.5" stroke-linejoin="round"/></svg>',
    styler:
      '<svg viewBox="0 0 28 28" width="28" height="28" aria-hidden="true"><rect width="28" height="28" rx="6" fill="#1565C0"/><path d="M9 18.5c0-3.2 3.4-4.4 3.4-7.2 0-1.5-1-2.6-2.4-2.6-1.6 0-2.5 1.3-2.5 2.8" fill="none" stroke="#fff" stroke-width="1.5" stroke-linecap="round"/><circle cx="18.4" cy="11.2" r="2.1" fill="#90CAF9"/><circle cx="16.2" cy="17.4" r="2.1" fill="#fff"/><circle cx="20.6" cy="17.4" r="2.1" fill="#BBDEFB"/></svg>',
    audit:
      '<svg viewBox="0 0 28 28" width="28" height="28" aria-hidden="true"><rect width="28" height="28" rx="6" fill="#0F4C75"/><circle cx="14" cy="14" r="6.2" fill="none" stroke="#7DCED6" stroke-width="1.8"/><circle cx="14" cy="14" r="2.2" fill="#7DCED6"/></svg>',
    geosuite:
      '<svg viewBox="0 0 28 28" width="28" height="28" aria-hidden="true"><rect width="28" height="28" rx="6" fill="#334155"/><rect x="7" y="16.5" width="14" height="3.2" rx="1" fill="#fff"/><rect x="8.2" y="12.4" width="11.6" height="3.2" rx="1" fill="#CBD5E1"/><rect x="9.4" y="8.3" width="9.2" height="3.2" rx="1" fill="#fff"/></svg>',
    qcarta:
      '<svg viewBox="0 0 28 28" width="28" height="28" aria-hidden="true"><rect width="28" height="28" rx="6" fill="#0D652D"/><path d="M14 6.8c-2.6 0-4.7 2-4.7 4.5 0 3.4 4.7 8.7 4.7 8.7s4.7-5.3 4.7-8.7c0-2.5-2.1-4.5-4.7-4.5z" fill="#fff"/><circle cx="14" cy="11.2" r="1.6" fill="#0D652D"/></svg>',
  };

  var ARROW =
    '<svg class="acugis-gn-arrow acugis-gn-card-arrow" viewBox="0 0 16 16" aria-hidden="true"><path d="M3 8h10M9 4l4 4-4 4" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round"/></svg>';

  var FOOT_ARROW =
    '<svg class="acugis-gn-arrow" viewBox="0 0 16 16" aria-hidden="true"><path d="M3 8h10M9 4l4 4-4 4" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round"/></svg>';

  var CARET =
    '<svg class="acugis-gn-caret" viewBox="0 0 12 12" aria-hidden="true"><path d="M2.5 4.5L6 8l3.5-3.5" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"/></svg>';

  var PRODUCTS = [
    {
      id: "report-server",
      name: "Report Server",
      category: "Reporting & Analytics",
      description: "Self-hosted reporting, analytics, dashboards and geospatial reporting.",
      href: "/report-server/",
      icon: "report-server",
      image: PRODUCT_ASSET_BASE + "report-server.png",
    },
    {
      id: "report-designer",
      name: "Report Designer",
      category: "Report Design",
      description: "Desktop design studio for maps, charts, and pixel-perfect reports.",
      href: "/report-designer/",
      icon: "report-designer",
      image: PRODUCT_ASSET_BASE + "report-designer.png",
    },
    {
      id: "atlas",
      name: "QCarta Survey",
      category: "Web GIS",
      description: "QGIS publishing, web maps, reports and dashboards.",
      href: "/QCarta-Survey/",
      icon: "qgis-publisher",
      image: PRODUCT_ASSET_BASE + "atlas.jpg",
    },
  ];

  var PRODUCT_LINKS = [
    { name: "Documentation", href: "/documentation/" },
    { name: "Quick Starts", href: "/documentation/" },
    { name: "Support", href: "/report-server/#contact" },
  ];

  var OPEN_SOURCE_GROUPS = [
    {
      label: "GeoServer Tools",
      projects: [
        {
          id: "drive",
          name: "Drive",
          description: "Geospatial data management and versioning for GeoServer.",
          href: "/drive/",
          icon: "drive",
        },
        {
          id: "styler",
          name: "Styler",
          description: "Visual style editor for GeoServer.",
          href: "/styler/",
          icon: "styler",
        },
        {
          id: "audit",
          name: "Audit",
          description: "GeoServer auditing and monitoring.",
          href: "/geosiem/",
          icon: "audit",
        },
      ],
    },
    {
      label: "Other Open Source Projects",
      projects: [
        {
          id: "geosuite",
          name: "GeoSuite",
          description: "Install and manage an open-source GIS stack.",
          href: "/geosuite/",
          icon: "geosuite",
        },
        {
          id: "qcarta",
          name: "QCarta",
          description: "QGIS map portal and web publishing.",
          href: "/QCarta/",
          icon: "qcarta",
        },
      ],
    },
  ];

  var RESOURCES = [
    { name: "Documentation", href: "/documentation/" },
    { name: "Quick Starts", href: "/documentation/" },
    { name: "Tutorials", href: "/enduser" },
    { name: "GitHub", href: "https://github.com/AcuGIS", external: true },
    { name: "Community", href: "https://github.com/AcuGIS", external: true },
    { name: "Blog", href: "https://acugis.com/blogs", external: true },
    { name: "Knowledge Base", href: "https://www.acugis.com/members/knowledgebase", external: true },
    { name: "Support", href: "/report-server/#contact" },
  ];

  var GITHUB = { name: "GitHub", href: "https://github.com/AcuGIS", external: true };

  var HOME = "/";
  var DOCS = "/documentation/";
  var PRICING = "/pricing/";
  var GET_STARTED = "/cloud/#benefits";

  function escapeHtml(value) {
    return String(value)
      .replace(/&/g, "&amp;")
      .replace(/</g, "&lt;")
      .replace(/>/g, "&gt;")
      .replace(/"/g, "&quot;");
  }

  function linkAttrs(item) {
    var attrs = 'href="' + escapeHtml(item.href) + '"';
    if (item.external) {
      attrs += ' target="_blank" rel="noopener noreferrer"';
    }
    return attrs;
  }

  function listLinks(items, className) {
    return items
      .map(function (item) {
        return (
          '<li><a class="' +
          className +
          '" ' +
          linkAttrs(item) +
          ">" +
          escapeHtml(item.name) +
          "</a></li>"
        );
      })
      .join("");
  }

  function productCard(product, activeId) {
    var isActive = product.id === activeId;
    return (
      '<a class="acugis-gn-card' +
      (isActive ? " is-active" : "") +
      '" data-accent="' +
      product.id +
      '" href="' +
      escapeHtml(product.href) +
      '"' +
      (isActive ? ' aria-current="page"' : "") +
      ">" +
      '<span class="acugis-gn-card-icon">' +
      ICONS[product.icon] +
      "</span>" +
      '<span class="acugis-gn-card-name">' +
      escapeHtml(product.name) +
      "</span>" +
      '<span class="acugis-gn-card-cat">' +
      escapeHtml(product.category) +
      "</span>" +
      '<span class="acugis-gn-card-media">' +
      '<img src="' +
      escapeHtml(product.image) +
      '" alt="" width="350" height="250">' +
      "</span>" +
      '<span class="acugis-gn-card-desc">' +
      escapeHtml(product.description) +
      "</span>" +
      ARROW +
      "</a>"
    );
  }

  function projectEntry(project) {
    var inner =
      '<span class="acugis-gn-project-icon">' +
      ICONS[project.icon] +
      "</span>" +
      '<span class="acugis-gn-project-copy">' +
      '<span class="acugis-gn-project-name">' +
      escapeHtml(project.name) +
      "</span>" +
      '<span class="acugis-gn-project-desc">' +
      escapeHtml(project.description) +
      "</span></span>";

    if (!project.href) {
      return '<div class="acugis-gn-project acugis-gn-project--static">' + inner + "</div>";
    }

    return '<a class="acugis-gn-project" ' + linkAttrs(project) + ">" + inner + "</a>";
  }

  function mobileProduct(product, activeId) {
    var isActive = product.id === activeId;
    return (
      '<a class="acugis-gn-mobile-product' +
      (isActive ? " is-active" : "") +
      '" href="' +
      escapeHtml(product.href) +
      '"' +
      (isActive ? ' aria-current="page"' : "") +
      ">" +
      '<span class="acugis-gn-mobile-product-icon">' +
      ICONS[product.icon] +
      "</span>" +
      "<span><span class=\"acugis-gn-mobile-product-name\">" +
      escapeHtml(product.name) +
      '</span><span class="acugis-gn-mobile-product-cat">' +
      escapeHtml(product.category) +
      "</span></span></a>"
    );
  }

  function mobileOpenSource(project) {
    if (!project.href) {
      return (
        '<span class="acugis-gn-mobile-link acugis-gn-mobile-link--static">' +
        escapeHtml(project.name) +
        "</span>"
      );
    }
    return (
      '<a class="acugis-gn-mobile-link" ' +
      linkAttrs(project) +
      ">" +
      escapeHtml(project.name) +
      "</a>"
    );
  }

  function render(root) {
    var activeId = root.getAttribute("data-product") || "";
    var mode = root.getAttribute("data-nav-mode") || "sticky";
    var path = window.location.pathname || "";
    var docsCurrent = path.indexOf("/documentation") === 0;
    var pricingCurrent = path === "/pricing" || path.indexOf("/pricing/") === 0;

    if (activeId === "qgis-publisher") activeId = "atlas";
    if (!activeId) {
      if (path === "/atlas-survey" || path.indexOf("/atlas-survey/") === 0) activeId = "";
      else if (
        path.toLowerCase() === "/qcarta-survey" ||
        path.toLowerCase().indexOf("/qcarta-survey/") === 0 ||
        path === "/atlas" ||
        path.indexOf("/atlas/") === 0 ||
        path.indexOf("/qgis-publisher") === 0
      )
        activeId = "atlas";
      else if (path === "/report-designer" || path.indexOf("/report-designer/") === 0) activeId = "report-designer";
      else if (path === "/report-server" || path.indexOf("/report-server/") === 0) activeId = "report-server";
      else if (path === "/drive" || path.indexOf("/drive/") === 0) activeId = "drive";
      else if (path === "/styler" || path.indexOf("/styler/") === 0) activeId = "styler";
      else if (path === "/geosuite" || path.indexOf("/geosuite/") === 0) activeId = "geosuite";
      else if (path.toLowerCase() === "/qcarta" || path.toLowerCase().indexOf("/qcarta/") === 0) activeId = "qcarta";
    }

    root.classList.add("acugis-gn");
    root.classList.remove("acugis-gn--overlay", "acugis-gn--static");
    if (mode === "overlay") root.classList.add("acugis-gn--overlay");
    if (mode === "static") root.classList.add("acugis-gn--static");
    if (!root.getAttribute("aria-label")) root.setAttribute("aria-label", "AcuGIS");

    var cards = PRODUCTS.map(function (product) {
      return productCard(product, activeId);
    }).join("");

    var mobileCards = PRODUCTS.map(function (product) {
      return mobileProduct(product, activeId);
    }).join("");

    var openSourceCols = OPEN_SOURCE_GROUPS.map(function (group) {
      return (
        "<div>" +
        '<p class="acugis-gn-col-label">' +
        escapeHtml(group.label) +
        "</p>" +
        '<div class="acugis-gn-projects">' +
        group.projects.map(projectEntry).join("") +
        "</div></div>"
      );
    }).join("");

    var mobileOpenSourceItems = OPEN_SOURCE_GROUPS.reduce(function (html, group) {
      return html + group.projects.map(mobileOpenSource).join("");
    }, "");

    root.innerHTML =
      '<div class="acugis-gn-bar">' +
      '<div class="acugis-gn-inner">' +
      '<a class="acugis-gn-logo" href="' +
      HOME +
      '">AcuGIS</a>' +
      '<div class="acugis-gn-desktop">' +
      '<ul class="acugis-gn-links">' +
      '<li class="acugis-gn-item acugis-gn-item--products" data-menu="products">' +
      '<button class="acugis-gn-trigger" type="button" aria-expanded="false" aria-haspopup="true" aria-controls="acugis-gn-panel-products" id="acugis-gn-btn-products">Products ' +
      CARET +
      "</button></li>" +
      '<li class="acugis-gn-item" data-menu="opensource">' +
      '<button class="acugis-gn-trigger" type="button" aria-expanded="false" aria-haspopup="true" aria-controls="acugis-gn-panel-opensource" id="acugis-gn-btn-opensource">Open Source ' +
      CARET +
      "</button></li>" +
      '<li class="acugis-gn-item" data-menu="resources">' +
      '<button class="acugis-gn-trigger" type="button" aria-expanded="false" aria-haspopup="true" aria-controls="acugis-gn-panel-resources" id="acugis-gn-btn-resources">Resources ' +
      CARET +
      "</button>" +
      '<div class="acugis-gn-dropdown" id="acugis-gn-panel-resources" role="region" aria-labelledby="acugis-gn-btn-resources" aria-hidden="true">' +
      '<ul class="acugis-gn-list">' +
      listLinks(RESOURCES, "") +
      "</ul></div></li>" +
      '<li class="acugis-gn-item">' +
      '<a class="acugis-gn-link' +
      (pricingCurrent ? " is-current" : "") +
      '" href="' +
      PRICING +
      '"' +
      (pricingCurrent ? ' aria-current="page"' : "") +
      ">Pricing</a></li>" +
      "</ul>" +
      '<div class="acugis-gn-actions">' +
      '<a class="acugis-gn-doc' +
      (docsCurrent ? " is-current" : "") +
      '" href="' +
      DOCS +
      '"' +
      (docsCurrent ? ' aria-current="page"' : "") +
      ">Documentation</a>" +
      '<a class="acugis-gn-cta" href="' +
      GET_STARTED +
      '">Get Started</a>' +
      "</div></div>" +
      '<button class="acugis-gn-burger" type="button" aria-expanded="false" aria-controls="acugis-gn-mobile" aria-label="Open menu">' +
      '<span class="acugis-gn-burger-bar"></span><span class="acugis-gn-burger-bar"></span><span class="acugis-gn-burger-bar"></span>' +
      "</button></div></div>" +
      '<div class="acugis-gn-panels">' +
      '<div class="acugis-gn-mega acugis-gn-mega--products" id="acugis-gn-panel-products" role="region" aria-labelledby="acugis-gn-btn-products" aria-hidden="true">' +
      '<div class="acugis-gn-mega-body acugis-gn-mega-body--products">' +
      '<p class="acugis-gn-col-label">Products</p>' +
      '<div class="acugis-gn-product-grid acugis-gn-product-grid--row">' +
      cards +
      "</div></div>" +
      '<div class="acugis-gn-mega-foot acugis-gn-mega-foot--links">' +
      "<ul>" +
      listLinks(PRODUCT_LINKS, "") +
      "</ul></div></div>" +
      '<div class="acugis-gn-mega acugis-gn-mega--opensource" id="acugis-gn-panel-opensource" role="region" aria-labelledby="acugis-gn-btn-opensource" aria-hidden="true">' +
      '<div class="acugis-gn-mega-body acugis-gn-mega-body--opensource">' +
      '<p class="acugis-gn-col-label">Open Source</p>' +
      '<div class="acugis-gn-os-grid">' +
      openSourceCols +
      "</div></div>" +
      '<div class="acugis-gn-mega-foot acugis-gn-mega-foot--os">' +
      '<p>Open-source GIS tools from AcuGIS.<span class="acugis-gn-mega-foot-sub">Commercial support available.</span></p>' +
      "<a " +
      linkAttrs(GITHUB) +
      ">GitHub " +
      FOOT_ARROW +
      "</a></div></div></div>" +
      '<div class="acugis-gn-mobile" id="acugis-gn-mobile">' +
      '<div class="acugis-gn-mobile-inner">' +
      '<div class="acugis-gn-accordion" data-accordion="products">' +
      '<button class="acugis-gn-accordion-btn" type="button" aria-expanded="false">Products <span class="acugis-gn-plus" aria-hidden="true"></span></button>' +
      '<div class="acugis-gn-accordion-panel">' +
      mobileCards +
      "</div></div>" +
      '<div class="acugis-gn-accordion" data-accordion="opensource">' +
      '<button class="acugis-gn-accordion-btn" type="button" aria-expanded="false">Open Source <span class="acugis-gn-plus" aria-hidden="true"></span></button>' +
      '<div class="acugis-gn-accordion-panel">' +
      mobileOpenSourceItems +
      "</div></div>" +
      '<div class="acugis-gn-accordion" data-accordion="resources">' +
      '<button class="acugis-gn-accordion-btn" type="button" aria-expanded="false">Resources <span class="acugis-gn-plus" aria-hidden="true"></span></button>' +
      '<div class="acugis-gn-accordion-panel"><ul>' +
      listLinks(RESOURCES, "acugis-gn-mobile-link") +
      "</ul></div></div>" +
      '<a class="acugis-gn-mobile-link' +
      (pricingCurrent ? " is-current" : "") +
      '" href="' +
      PRICING +
      '"' +
      (pricingCurrent ? ' aria-current="page"' : "") +
      ">Pricing</a>" +
      '<a class="acugis-gn-mobile-link' +
      (docsCurrent ? " is-current" : "") +
      '" href="' +
      DOCS +
      '"' +
      (docsCurrent ? ' aria-current="page"' : "") +
      ">Documentation</a>" +
      '<div class="acugis-gn-mobile-cta"><a class="acugis-gn-cta" href="' +
      GET_STARTED +
      '">Get Started</a></div>' +
      "</div></div>";

    bind(root);
  }

  function bind(root) {
    var closeTimer = null;
    var openMenuId = null;
    var openMethod = null;
    var burger = root.querySelector(".acugis-gn-burger");
    var mobile = root.querySelector("#acugis-gn-mobile");
    var items = root.querySelectorAll(".acugis-gn-item[data-menu]");

    function panelFor(id) {
      return root.querySelector("#acugis-gn-panel-" + id);
    }

    function setExpanded(id, expanded) {
      var item = root.querySelector('.acugis-gn-item[data-menu="' + id + '"]');
      var trigger = item ? item.querySelector(".acugis-gn-trigger") : null;
      var panel = panelFor(id);
      if (item) item.classList.toggle("is-open", expanded);
      if (trigger) trigger.setAttribute("aria-expanded", expanded ? "true" : "false");
      if (panel) {
        panel.setAttribute("aria-hidden", expanded ? "false" : "true");
        panel.classList.toggle("is-visible", expanded);
      }
    }

    function closeDesktop() {
      if (!openMenuId) return;
      setExpanded(openMenuId, false);
      root.classList.remove("is-open");
      openMenuId = null;
      openMethod = null;
    }

    function openDesktop(id, method) {
      if (openMenuId && openMenuId !== id) setExpanded(openMenuId, false);
      openMenuId = id;
      openMethod = method;
      root.classList.add("is-open");
      setExpanded(id, true);
    }

    function scheduleClose() {
      window.clearTimeout(closeTimer);
      closeTimer = window.setTimeout(function () {
        if (openMethod === "keyboard") return;
        closeDesktop();
      }, 140);
    }

    function cancelClose() {
      window.clearTimeout(closeTimer);
    }

    items.forEach(function (item) {
      var id = item.getAttribute("data-menu");
      var trigger = item.querySelector(".acugis-gn-trigger");
      var panel = panelFor(id);

      trigger.addEventListener("mouseenter", function () {
        if (window.matchMedia("(max-width: 1023px)").matches) return;
        cancelClose();
        openDesktop(id, "hover");
      });

      trigger.addEventListener("click", function (event) {
        event.preventDefault();
        if (openMenuId === id) {
          closeDesktop();
          return;
        }
        openDesktop(id, "click");
      });

      trigger.addEventListener("keydown", function (event) {
        if (event.key === "ArrowDown" || event.key === "Enter" || event.key === " ") {
          event.preventDefault();
          openDesktop(id, "keyboard");
          var first = panel && panel.querySelector("a");
          if (first) first.focus();
        }
        if (event.key === "Tab" && !event.shiftKey && openMenuId === id) {
          var firstLink = panel && panel.querySelector("a");
          if (firstLink) {
            event.preventDefault();
            firstLink.focus();
          }
        }
      });
    });

    root.addEventListener("mouseenter", cancelClose);
    root.addEventListener("mouseleave", function () {
      if (window.matchMedia("(max-width: 1023px)").matches) return;
      scheduleClose();
    });

    document.addEventListener("click", function (event) {
      if (!root.contains(event.target)) {
        closeDesktop();
        closeMobile();
      }
    });

    document.addEventListener("keydown", function (event) {
      if (event.key !== "Escape") return;
      if (root.classList.contains("is-mobile-open")) {
        closeMobile();
        burger.focus();
        return;
      }
      if (openMenuId) {
        var trigger = root.querySelector('.acugis-gn-item[data-menu="' + openMenuId + '"] .acugis-gn-trigger');
        closeDesktop();
        if (trigger) trigger.focus();
      }
    });

    function setMobileOpen(open) {
      root.classList.toggle("is-mobile-open", open);
      burger.setAttribute("aria-expanded", open ? "true" : "false");
      burger.setAttribute("aria-label", open ? "Close menu" : "Open menu");
      document.body.classList.toggle("acugis-gn-lock", open);
    }

    function closeMobile() {
      setMobileOpen(false);
    }

    burger.addEventListener("click", function () {
      setMobileOpen(!root.classList.contains("is-mobile-open"));
    });

    root.querySelectorAll(".acugis-gn-accordion-btn").forEach(function (button) {
      button.addEventListener("click", function () {
        var accordion = button.closest(".acugis-gn-accordion");
        var willOpen = !accordion.classList.contains("is-open");
        accordion.classList.toggle("is-open", willOpen);
        button.setAttribute("aria-expanded", willOpen ? "true" : "false");
      });
    });

    if (mobile) {
      mobile.querySelectorAll("a").forEach(function (link) {
        link.addEventListener("click", closeMobile);
      });
    }

    root.querySelectorAll(".acugis-gn-mega a, .acugis-gn-dropdown a").forEach(function (link) {
      link.addEventListener("click", closeDesktop);
    });

    window.addEventListener("resize", function () {
      if (!window.matchMedia("(max-width: 1023px)").matches) {
        closeMobile();
      } else {
        closeDesktop();
      }
    });
  }

  function init() {
    var roots = document.querySelectorAll("[data-acugis-nav]");
    roots.forEach(render);
  }

  if (document.readyState === "loading") {
    document.addEventListener("DOMContentLoaded", init);
  } else {
    init();
  }
})();
