/**
 * Federation platform shell — global sidebar, workspace pages, in-content app nav.
 */
(function (global) {
  'use strict';

  var config = global.FEDERATION_SHELL_CONFIG;
  if (!config) return;

  function assetsBase() {
    if (global.FEDERATION_SHELL_ASSETS) return String(global.FEDERATION_SHELL_ASSETS).replace(/\/$/, '');
    return String(config.assetsBase || '/shared/ui').replace(/\/$/, '');
  }

  function pathname() {
    return (global.location && global.location.pathname) || '';
  }

  function detectAppId() {
    var body = document.body;
    if (body && body.getAttribute('data-federation-app')) {
      return body.getAttribute('data-federation-app');
    }
    if (isCentralServicesPath()) {
      return 'central';
    }
    var path = pathname().toLowerCase();
    var apps = config.apps || {};
    for (var id in apps) {
      if (!Object.prototype.hasOwnProperty.call(apps, id)) continue;
      var app = apps[id];
      var matches = app.match || [];
      for (var i = 0; i < matches.length; i++) {
        if (path.indexOf(String(matches[i]).toLowerCase()) !== -1) return id;
      }
    }
    if (path.indexOf('/admin') !== -1) return 'administration';
    return '';
  }

  function appConfig(appId) {
    return (config.apps && config.apps[appId]) || null;
  }

  function joinMount(mount, path) {
    var m = String(mount || '').replace(/\/$/, '');
    var p = String(path || '');
    if (!p) return m || '/';
    if (p.charAt(0) !== '/') p = '/' + p;
    return m ? m + p : p;
  }

  /** Pathname with query/hash, .php, and /index removed so PHP and Go URLs compare equal. */
  function canonicalNavPath(path) {
    var p = String(path || '');
    var hash = p.indexOf('#');
    if (hash !== -1) p = p.slice(0, hash);
    var q = p.indexOf('?');
    if (q !== -1) p = p.slice(0, q);
    if (p.indexOf('://') !== -1) {
      try {
        p = new URL(p, global.location.origin).pathname;
      } catch (_e) {}
    }
    p = p.replace(/\/+$/, '') || '/';
    p = p.replace(/\.php$/i, '');
    if (/\/index$/i.test(p)) p = p.replace(/\/index$/i, '') || '/';
    return p.toLowerCase();
  }

  function relativeAppPath(app) {
    var cur = canonicalNavPath(pathname());
    var mount = canonicalNavPath((app && app.mount) || '');
    if (mount && mount !== '/' && (cur === mount || cur.indexOf(mount + '/') === 0)) {
      cur = cur.slice(mount.length) || '/';
      if (cur.charAt(0) !== '/') cur = '/' + cur;
    }
    return cur || '/';
  }

  /** rulePath matches cur at a path boundary, including when cur has a mount prefix. */
  function pathWithin(rulePath, cur) {
    if (!rulePath || rulePath === '/' || !cur) return false;
    var from = 0;
    while (from < cur.length) {
      var at = cur.indexOf(rulePath, from);
      if (at === -1) return false;
      var after = at + rulePath.length;
      var afterOk = after === cur.length || cur.charAt(after) === '/';
      // A rule like /admin already contains the slash, so /go/admin/license matches.
      var beforeOk = rulePath.charAt(0) === '/' ? cur.charAt(at) === '/' : at === 0 || cur.charAt(at - 1) === '/';
      if (beforeOk && afterOk) return true;
      from = at + 1;
    }
    return false;
  }

  // Used when an older shell-config.js has no per-hub owns list.
  var BUILTIN_HUB_OWNS = {
    reports: ['/reports'],
    'data-explorer': ['/explorer'],
    scheduling: ['/schedules', '/edit_schedule', '/executions', '/history', '/recipients', '/edit_recipient'],
    data: [
      '/reports/datasources',
      '/reports/edit_datasource',
      '/reports/geoserver_connections',
      '/reports/qgis_connections',
      '/reports/report_runner',
      '/reports/generate_jasper_context',
    ],
    administration: ['/admin'],
  };

  function ownsRuleMatch(rule, relPath) {
    var raw = String(rule || '');
    var exact = raw.charAt(raw.length - 1) === '$';
    if (exact) raw = raw.slice(0, -1);
    var rulePath = canonicalNavPath(raw);
    if (!rulePath || rulePath === '/') return 0;
    if (exact) {
      if (relPath === rulePath) return rulePath.length;
      var at = relPath.lastIndexOf(rulePath);
      if (at >= 0 && at + rulePath.length === relPath.length && (at === 0 || relPath.charAt(at - 1) === '/')) {
        return rulePath.length;
      }
      return 0;
    }
    // pathWithin still matches when the app is mounted under a prefix.
    if (relPath === rulePath || relPath.indexOf(rulePath + '/') === 0 || pathWithin(rulePath, relPath)) {
      return rulePath.length;
    }
    return 0;
  }

  function bestOwnedHub(app) {
    if (!app || !app.hubs) return null;
    var rel = relativeAppPath(app);
    var best = null;
    var bestLen = 0;
    for (var key in app.hubs) {
      if (!Object.prototype.hasOwnProperty.call(app.hubs, key)) continue;
      var hub = app.hubs[key];
      var owns = hub.owns && hub.owns.length ? hub.owns : BUILTIN_HUB_OWNS[key] || [];
      for (var i = 0; i < owns.length; i++) {
        var len = ownsRuleMatch(owns[i], rel);
        if (len > bestLen) {
          bestLen = len;
          best = { id: key, config: hub };
        }
      }
    }
    return best;
  }

  function isWorkspaceTarget(href) {
    var p = canonicalNavPath(href);
    return p === '/workspace' || /\/workspace$/.test(p);
  }

  /** Section landing used when a page has no closer hub. Never /workspace. */
  function sectionHomeHref(app) {
    if (config.portalHome && !isWorkspaceTarget(config.portalHome)) return config.portalHome;
    if (app && app.home && !isWorkspaceTarget(app.home)) return joinMount(app.mount, app.home);
    return joinMount(app && app.mount, '/hubs/reports.php');
  }

  /**
   * Go serves extensionless URLs and redirects *.php. Keep .php links on PHP pages.
   */
  function cleanPublicHref(href) {
    var here = pathname();
    if (/\.php(?:$|[?#])/i.test(here)) return href;
    return String(href || '')
      .replace(/\/index\.php(?=$|[?#])/i, '/')
      .replace(/\.php(?=$|[?#])/i, '');
  }

  /** Full platform URL for global sidebar (reverse-proxy mount + app path). */
  function globalAppHref(app) {
    if (!app) return '#';
    var target = app.workspace || app.home || '/';
    if (isWorkspaceTarget(target)) target = '/hubs/reports.php';
    return joinMount(app.mount, target);
  }

  function hubPathname(app, hub) {
    var hubPath = appUrl(app, hub.path);
    try {
      return (hubPath.indexOf('://') !== -1
        ? new URL(hubPath, global.location.origin).pathname
        : hubPath
      ).replace(/\/$/, '') || '/';
    } catch (_e) {
      return String(hub.path || '').replace(/\/$/, '') || '/';
    }
  }

  function detectHubPage(app) {
    if (!app || !app.hubs) return null;
    var bodyHub = document.body && document.body.getAttribute('data-federation-hub');
    if (bodyHub && app.hubs[bodyHub]) {
      return { id: bodyHub, config: app.hubs[bodyHub] };
    }
    var mainHub = document.querySelector('main[data-federation-hub]');
    if (mainHub) {
      var mainId = mainHub.getAttribute('data-federation-hub');
      if (mainId && app.hubs[mainId]) {
        return { id: mainId, config: app.hubs[mainId] };
      }
    }
    var path = canonicalNavPath(pathname());
    for (var key in app.hubs) {
      if (!Object.prototype.hasOwnProperty.call(app.hubs, key)) continue;
      var hub = app.hubs[key];
      if (path === canonicalNavPath(hubPathname(app, hub))) {
        return { id: key, config: hub };
      }
    }
    return null;
  }

  function detectHubParent(app) {
    if (!app || !app.hubs) return null;
    var owned = bestOwnedHub(app);
    if (owned) return owned;
    var path = canonicalNavPath(pathname());
    for (var key in app.hubs) {
      if (!Object.prototype.hasOwnProperty.call(app.hubs, key)) continue;
      var hub = app.hubs[key];
      var hubPath = canonicalNavPath(hubPathname(app, hub));
      if (path !== hubPath && path.indexOf(hubPath + '/') === 0) {
        return { id: key, config: hub };
      }
      var sections = hub.sections || [];
      for (var s = 0; s < sections.length; s++) {
        var items = sections[s].items || [];
        for (var i = 0; i < items.length; i++) {
          if (activeItemHref(app, items[i])) {
            return { id: key, config: hub };
          }
        }
      }
    }
    return null;
  }

  function parentNavHref(app) {
    var hubParent = detectHubParent(app);
    var href = hubParent ? appUrl(app, hubParent.config.path) : sectionHomeHref(app);
    return cleanPublicHref(href);
  }

  function appUrl(app, href) {
    if (!href) return '#';
    if (app && typeof app.urlPrefix === 'function') {
      var resolved = app.urlPrefix(href);
      if (resolved && resolved !== href) return resolved;
      if (app.mount && href.charAt(0) === '/' && href.indexOf(app.mount) !== 0) {
        return joinMount(app.mount, href);
      }
      return resolved;
    }
    if (app && app.mount) return joinMount(app.mount, href);
    return href;
  }

  function isCentralServicesPath() {
    var path = pathname().replace(/\/$/, '') || '/';
    return path === '/services' || path.endsWith('/services');
  }

  /** Central portal home — full-width layout with top nav only (no app-wide sidebar). */
  function isPortalHomePage() {
    var path = pathname().replace(/\/$/, '') || '/';
    if (path !== '/' && !path.endsWith('/index.html')) return false;
    if (isCentralServicesPath()) return false;
    if (document.querySelector('main[data-federation-workspace]')) return false;
    return !!(document.querySelector('main.portal-main') && document.querySelector('nav.topnav'));
  }

  function isShellDisabled() {
    var body = document.body;
    if (!body) return false;
    if (body.hasAttribute('data-federation-no-shell')) return true;
    return isPortalHomePage();
  }

  function isNoSidebarPage() {
    var body = document.body;
    return !!(body && body.hasAttribute('data-federation-no-sidebar'));
  }

  function isNoBackPage() {
    var body = document.body;
    return !!(body && body.hasAttribute('data-federation-no-back'));
  }

  function clearShellChrome() {
    var body = document.body;
    if (!body) return;
    body.classList.remove('fed-shell-body', 'fed-shell-open', 'fed-central-workspace');
    document.documentElement.classList.remove('fed-shell-ready');
    var aside = document.getElementById('fed-shell-sidebar');
    if (aside) aside.remove();
    document.querySelectorAll('.fed-shell-menu-btn, .fed-shell-backdrop').forEach(function (el) {
      el.remove();
    });
    var topnav = document.querySelector('nav.topnav');
    if (topnav) topnav.style.display = '';
    var footer = document.querySelector('.portal-footer');
    if (footer) footer.style.display = '';
  }

  function clearSidebarChrome() {
    var aside = document.getElementById('fed-shell-sidebar');
    if (aside) aside.remove();
    document.querySelectorAll('.fed-shell-menu-btn, .fed-shell-backdrop').forEach(function (el) {
      el.remove();
    });
    var body = document.body;
    if (body) body.classList.remove('fed-shell-open');
  }

  /** App-supplied workspace landing (e.g. GeoVault My Drive) — must not be replaced by renderWorkspace. */
  function isStaticWorkspaceContent() {
    var body = document.body;
    if (!body) return false;
    if (body.getAttribute('data-federation-workspace-static') === 'true') return true;
    if (body.getAttribute('data-page') === 'drive-home') return true;
    var main = document.querySelector(
      'main.main-content, main.fed-shell-main, main[data-federation-workspace], main.portal-main'
    );
    if (main && main.getAttribute('data-federation-workspace-static') === 'true') return true;
    if (main && main.querySelector('#btnNewFolder, #folderList, [data-gv-drive-home]')) return true;
    return false;
  }

  function isWorkspacePage(app) {
    if (!app) return false;
    if (isStaticWorkspaceContent()) return false;
    if (detectHubPage(app)) return false;
    if (app.id === 'central' && isCentralServicesPath()) return true;
    if (document.querySelector('main[data-federation-hub]')) return false;
    if (document.querySelector('main[data-federation-workspace], [data-federation-workspace].fed-shell-page')) return true;
    if (document.body && document.body.hasAttribute('data-federation-workspace')) return true;
    var path = pathname().replace(/\/$/, '') || '/';
    var ws = String(app.workspace || app.home || '').replace(/\/$/, '');
    if (!ws) return false;
    var full = globalAppHref(app);
    try {
      var fullPath = (full.indexOf('://') !== -1 ? new URL(full, global.location.origin).pathname : full).replace(/\/$/, '') || '/';
      return path === fullPath || path.endsWith(ws) || path.endsWith('/workspace');
    } catch (_e) {
      return path.indexOf('workspace') !== -1;
    }
  }

  function findWorkspaceMain() {
    return document.querySelector(
      'main.fed-shell-main, main[data-federation-workspace], main.portal-main, main.main-content, main.container'
    );
  }

  function hideLegacyPortalChrome() {
    var topnav = document.querySelector('nav.topnav');
    if (topnav) topnav.style.display = 'none';
    var footer = document.querySelector('.portal-footer');
    if (footer) footer.style.display = 'none';
  }

  function pathMatches(rule, path) {
    if (!rule) return false;
    var cur = canonicalNavPath(path);
    var raw = String(path || '');
    if (rule.charAt(0) === '^' || rule.indexOf('(?:') !== -1) {
      try {
        var re = new RegExp(rule, 'i');
        return re.test(raw) || re.test(cur);
      } catch (_e) {
        return false;
      }
    }
    return pathWithin(canonicalNavPath(rule), cur);
  }

  function activeItemHref(app, item) {
    var path = pathname();
    if (item.pathMatch && pathMatches(item.pathMatch, path)) return true;
    var itemPath = canonicalNavPath(appUrl(app, item.href));
    var cur = canonicalNavPath(path);
    if (!itemPath || itemPath === '/') return false;
    if (cur === itemPath) return true;
    // A section root such as /reports (from reports/index.php) must not claim
    // every URL underneath it. Count segments after the mount so a prefixed
    // deploy does not treat /reports as a deep path.
    var relItem = itemPath;
    var mount = canonicalNavPath((app && app.mount) || '');
    if (mount && mount !== '/' && (relItem === mount || relItem.indexOf(mount + '/') === 0)) {
      relItem = relItem.slice(mount.length) || '/';
    }
    var parts = relItem.split('/').filter(Boolean);
    if (parts.length < 2) return false;
    return pathWithin(itemPath, cur);
  }

  // Professional-only top-level items. Community Edition (and OSS builds)
  // must never show these, including on PHP/inner pages that paint nav in JS.
  var PROFESSIONAL_APP_NAV = {
    'Ad Hoc': 'dataExplorer',
    Dashboards: 'dashboards',
    'Data Quality': 'dataIntelligence',
    Auditing: 'audit',
  };

  function editionName() {
    return String((global.FEDERATION_EDITION_CAPS || {}).edition || '').toLowerCase();
  }

  function isCommunityEdition() {
    var ed = editionName();
    return ed === '' || ed === 'community' || ed === 'oss' || ed === 'ce' || ed === 'open source';
  }

  function editionFeatures() {
    var caps = global.FEDERATION_EDITION_CAPS || {};
    return caps.features || {};
  }

  function featureEnabled(name) {
    if (!name) return true;
    return editionFeatures()[name] === true;
  }

  function itemVisible(item) {
    if (item.requires) {
      var flags = global.FEDERATION_SHELL_FLAGS || {};
      if (flags[item.requires] !== true) return false;
    }
    if (item.flag && global.FEDERATION_SHELL_FLAGS && global.FEDERATION_SHELL_FLAGS[item.flag] === false) {
      return false;
    }
    var feature = item.feature || PROFESSIONAL_APP_NAV[item.label];
    if (feature) {
      if (isCommunityEdition() || !featureEnabled(feature)) return false;
    }
    return true;
  }

  function navItemDescription(item) {
    return item.description || item.desc || '';
  }

  function buildNavLinkInner(app, item, opts) {
    opts = opts || {};
    var desc = navItemDescription(item);
    var chevron = opts.external ? 'open_in_new' : 'chevron_right';
    var html =
      '<i class="material-icons">' +
      item.icon +
      '</i><span class="fed-app-nav-text"><span class="fed-app-nav-label">' +
      item.label +
      '</span>';
    if (desc) {
      html += '<span class="fed-app-nav-desc">' + desc + '</span>';
    }
    html += '</span><i class="material-icons fed-app-nav-chevron">' + chevron + '</i>';
    return html;
  }

  function buildAppNavSections(app, opts) {
    opts = opts || {};
    var html = '';
    for (var s = 0; s < (app.sections || []).length; s++) {
      var section = app.sections[s];
      var visibleItems = [];
      for (var i = 0; i < (section.items || []).length; i++) {
        if (itemVisible(section.items[i])) visibleItems.push(section.items[i]);
      }
      if (!visibleItems.length) continue;
      html += '<div class="fed-app-nav-section"><h2 class="fed-app-nav-section-title">' + section.title + '</h2>';
      html += '<div class="fed-app-nav-card"><ul class="fed-app-nav-list browser-default">';
      for (var v = 0; v < visibleItems.length; v++) {
        var item = visibleItems[v];
        var active = opts.markActive && activeItemHref(app, item);
        var href = cleanPublicHref(appUrl(app, item.href));
        var attrs = ' href="' + href + '"';
        if (item.external || opts.external) {
          attrs += ' target="_blank" rel="noopener"';
        }
        html +=
          '<li' +
          (active ? ' class="is-active"' : '') +
          '><a' +
          attrs +
          '>' +
          buildNavLinkInner(app, item, { external: item.external || opts.external }) +
          '</a></li>';
      }
      html += '</ul></div></div>';
    }
    return html;
  }

  function ensureStylesheet() {
    if (document.getElementById('fed-shell-css')) return;
    var link = document.createElement('link');
    link.id = 'fed-shell-css';
    link.rel = 'stylesheet';
    link.href = assetsBase() + '/shell.css';
    document.head.appendChild(link);
  }

  function ensureRobotoFont() {
    if (document.getElementById('fed-shell-roboto')) return;
    var existing = document.querySelector(
      'link[href*="fonts.googleapis.com/css2?family=Roboto"], link[href*="fonts.googleapis.com/css?family=Roboto"]'
    );
    if (existing) return;
    // Optional: if Roboto isn't in <head>, load without forcing a mid-session swap.
    var link = document.createElement('link');
    link.id = 'fed-shell-roboto';
    link.rel = 'stylesheet';
    link.href =
      'https://fonts.googleapis.com/css2?family=Roboto:wght@400;500&display=optional';
    document.head.appendChild(link);
  }

  var NAV_ICON_TINTS = {
    logout: 'grey',
    home: 'blue',
    admin_panel_settings: 'blue',
    map: 'green',
    folder: 'blue',
    storage: 'blue',
    public: 'teal',
    assessment: 'purple',
    schedule: 'purple',
    dashboard: 'orange',
    sync: 'pink',
    assignment: 'grey',
  };

  function navIconTint(item) {
    if (item.tint) return item.tint;
    return NAV_ICON_TINTS[item.icon] || 'blue';
  }

  function buildShellNavLink(href, item, opts) {
    opts = opts || {};
    href = cleanPublicHref(href);
    var tint = navIconTint(item);
    var html =
      '<a href="' +
      href +
      '"' +
      (opts.external ? ' target="_blank" rel="noopener"' : '') +
      '><span class="fed-shell-nav-icon fed-shell-nav-icon--' +
      tint +
      '" aria-hidden="true"><i class="material-icons">' +
      item.icon +
      '</i></span><span class="fed-shell-nav-label">' +
      item.label +
      '</span></a>';
    return html;
  }

  function explicitAppNavClaim() {
    var appNav = config.appNav || [];
    var path = pathname();
    for (var i = 0; i < appNav.length; i++) {
      var an = appNav[i];
      if (!itemVisible(an) || !an.match) continue;
      for (var m = 0; m < an.match.length; m++) {
        if (pathMatches(an.match[m], path)) return true;
      }
    }
    return false;
  }

  function hubNavMatches(app, hubId) {
    if (!app || !hubId || !app.hubs || !app.hubs[hubId]) return false;
    var hub = app.hubs[hubId];
    var hubPage = detectHubPage(app);
    if (hubPage && hubPage.id === hubId) return true;
    var owned = bestOwnedHub(app);
    if (owned && owned.id === hubId && !explicitAppNavClaim()) return true;
    var sections = hub.sections || [];
    for (var s = 0; s < sections.length; s++) {
      var items = sections[s].items || [];
      for (var i = 0; i < items.length; i++) {
        // Items promoted to top-level appNav (activateHub: false) should not
        // keep their parent hub highlighted.
        if (items[i].activateHub === false) continue;
        if (activeItemHref(app, items[i])) return true;
      }
    }
    return false;
  }

  function appNavHref(an) {
    var ac = appConfig(an.appId);
    if (an.href) return ac ? appUrl(ac, an.href) : an.href;
    if (an.hubId && ac && ac.hubs && ac.hubs[an.hubId]) {
      return appUrl(ac, ac.hubs[an.hubId].path);
    }
    return ac ? globalAppHref(ac) : '#';
  }

  function appNavItemActive(an, activeAppId) {
    if (an.disabled) return false;
    var path = pathname();
    if (an.match) {
      for (var m = 0; m < an.match.length; m++) {
        if (pathMatches(an.match[m], path)) return true;
      }
      // Explicit path matchers are authoritative: do not fall through to
      // hub/href/appId checks (that incorrectly marks Dashboards/DI active
      // on every jasper page).
      return false;
    }
    var ac = appConfig(an.appId);
    if (an.hubId) {
      return hubNavMatches(ac, an.hubId);
    }
    if (an.href) {
      try {
        var resolved = ac ? appUrl(ac, an.href) : an.href;
        var hrefPath = (resolved.indexOf('://') !== -1
          ? new URL(resolved, global.location.origin).pathname
          : resolved
        ).replace(/\/$/, '') || '/';
        var cur = path.replace(/\/$/, '') || '/';
        if (cur === hrefPath || cur.indexOf(hrefPath + '/') === 0) return true;
      } catch (_e) {
        if (path.indexOf(an.href) !== -1) return true;
      }
      return false;
    }
    return an.appId === activeAppId;
  }

  function buildGlobalNav(activeAppId) {
    var html = '';
    var globalNav = config.globalNav || [];
    var path = pathname();

    html += '<div class="fed-shell-nav-main">';

    if (globalNav.length) {
      html += '<div class="fed-shell-nav-section-label">Platform</div><ul class="fed-shell-nav-list browser-default">';
      for (var g = 0; g < globalNav.length; g++) {
        var gn = globalNav[g];
        var gActive = false;
        var gHref = gn.href || '/';
        if (gn.id === 'administration') {
          gActive = path.indexOf('/administration') !== -1 || path.indexOf('/services') !== -1;
        } else if (gn.match) {
          for (var gm = 0; gm < gn.match.length; gm++) {
            if (pathMatches(gn.match[gm], path)) {
              gActive = true;
              break;
            }
          }
        }
        html += '<li' + (gActive ? ' class="is-active"' : '') + '>' + buildShellNavLink(gHref, gn) + '</li>';
      }
      html += '</ul>';
    }

    html += '<div class="fed-shell-nav-section-label">Menu</div><ul class="fed-shell-nav-list browser-default">';
    var appNav = config.appNav || [];
    for (var a = 0; a < appNav.length; a++) {
      var an = appNav[a];
      if (!itemVisible(an)) continue;
      var href = appNavHref(an);
      var active = appNavItemActive(an, activeAppId);
      var cls = active ? ' class="is-active"' : '';
      if (an.disabled) cls = ' class="is-disabled"';
      html +=
        '<li' +
        cls +
        '>' +
        buildShellNavLink(href, an, { external: !!an.external }) +
        '</li>';
    }
    html += '</ul></div>';

    var footerNav = config.footerNav || [];
    if (footerNav.length) {
      html += '<div class="fed-shell-nav-footer"><ul class="fed-shell-nav-list browser-default">';
      for (var f = 0; f < footerNav.length; f++) {
        var fn = footerNav[f];
        html += '<li>' + buildShellNavLink(fn.href || '#', fn) + '</li>';
      }
      html += '</ul></div>';
    }

    return html;
  }

  function ensureShellChrome(activeAppId) {
    var body = document.body;
    if (!body) return;

    body.classList.add('fed-shell-body');
    document.documentElement.classList.add('fed-shell-ready');

    var aside = document.getElementById('fed-shell-sidebar');
    if (!aside) {
      aside = document.createElement('aside');
      aside.id = 'fed-shell-sidebar';
      aside.className = 'fed-shell-sidebar';
      aside.setAttribute('aria-label', 'Platform navigation');
      aside.innerHTML =
        '<a class="fed-shell-sidebar-brand" href="' +
        (config.portalHome || '/') +
        '">AcuGIS</a>' +
        '<div class="fed-shell-nav-scroll" data-fed-shell-nav-mount></div>';
      body.insertBefore(aside, body.firstChild);
    }

    var scroll = aside.querySelector('[data-fed-shell-nav-mount]') || aside.querySelector('.fed-shell-nav-scroll');
    if (scroll && scroll.getAttribute('data-fed-ssr') !== '1') {
      scroll.innerHTML = buildGlobalNav(activeAppId);
    }

    var backdrop = document.querySelector('.fed-shell-backdrop');
    if (!backdrop) {
      backdrop = document.createElement('div');
      backdrop.className = 'fed-shell-backdrop';
      body.insertBefore(backdrop, body.firstChild);
    }
    if (!backdrop.getAttribute('data-fed-shell-bound')) {
      backdrop.setAttribute('data-fed-shell-bound', '1');
      backdrop.addEventListener('click', function () {
        body.classList.remove('fed-shell-open');
      });
    }

    var btn = document.querySelector('.fed-shell-menu-btn');
    if (!btn) {
      btn = document.createElement('button');
      btn.type = 'button';
      btn.className = 'fed-shell-menu-btn';
      btn.setAttribute('aria-label', 'Open menu');
      btn.hidden = true;
      btn.innerHTML = '<i class="material-icons">menu</i>';
      body.insertBefore(btn, body.firstChild);
    }
    if (!btn.getAttribute('data-fed-shell-bound')) {
      btn.setAttribute('data-fed-shell-bound', '1');
      btn.addEventListener('click', function () {
        body.classList.toggle('fed-shell-open');
      });
    }

    // Close the mobile drawer as soon as a nav link is activated (avoids a
    // brief open-drawer paint before the next page's critical CSS applies).
    if (!aside.getAttribute('data-fed-shell-nav-bound')) {
      aside.setAttribute('data-fed-shell-nav-bound', '1');
      aside.addEventListener('click', function (ev) {
        var t = ev.target;
        while (t && t !== aside) {
          if (t.tagName === 'A' && t.getAttribute('href')) {
            body.classList.remove('fed-shell-open');
            break;
          }
          t = t.parentNode;
        }
      });
    }
  }

  function mountSidebar(activeAppId) {
    ensureShellChrome(activeAppId);
  }

  function resolvePageHeaderMeta(app) {
    if (!app) return null;
    var body = document.body;
    if (!body) return null;

    var title = body.getAttribute('data-page-title');
    var subtitle = body.getAttribute('data-page-subtitle');
    if (title) {
      return { title: title, subtitle: subtitle || '' };
    }

    var pageKey = body.getAttribute('data-page') || '';
    if (app.pageHeaders && app.pageHeaders[pageKey]) {
      return app.pageHeaders[pageKey];
    }
    return null;
  }

  function findAppMain() {
    return document.querySelector(
      'main.main-content, main.fed-shell-main, main.container.main-wrap, main.container, main.portal-main'
    );
  }

  /** Back arrow + layout for app feature pages (not workspace). */
  function enhanceAppPageHeader(app) {
    if (!app || isWorkspacePage(app) || isStaticWorkspaceContent()) return;

    var main = findAppMain();
    if (!main) return;

    var header = main.querySelector(':scope > .page-header, :scope > header.page-header');
    if (!header) {
      header = document.querySelector(
        'main.main-content .page-header, main.fed-shell-main .page-header, main.container .page-header, main .fed-shell-page-header'
      );
    }

    if (!header) {
      var meta = resolvePageHeaderMeta(app);
      if (!meta) return;
      header = document.createElement('header');
      header.className = 'page-header';
      var titleEl = document.createElement('h1');
      titleEl.className = 'page-title';
      titleEl.textContent = meta.title;
      header.appendChild(titleEl);
      if (meta.subtitle) {
        var subtitleEl = document.createElement('p');
        subtitleEl.className = 'page-subtitle';
        subtitleEl.textContent = meta.subtitle;
        header.appendChild(subtitleEl);
      }
      main.insertBefore(header, main.firstChild);
    }

    if (!header || header.classList.contains('fed-page-header-enhanced')) return;

    var bodyWrap = document.createElement('div');
    bodyWrap.className = 'fed-page-header-body';
    var hubParent = detectHubParent(app);
    if (hubParent && !isTopLevelHub(app, hubParent.id)) {
      var crumb = document.createElement('p');
      crumb.className = 'fed-hub-breadcrumb';
      crumb.innerHTML =
        '<a href="' +
        parentNavHref(app) +
        '">' +
        (hubParent.config.parentLabel || app.label) +
        '</a><span class="fed-hub-breadcrumb-sep"> / </span><span>' +
        hubParent.config.label +
        '</span>';
      bodyWrap.appendChild(crumb);
    }
    while (header.firstChild) {
      bodyWrap.appendChild(header.firstChild);
    }

    header.classList.add('fed-page-header', 'fed-page-header-enhanced');
    if (!isNoBackPage()) {
      var back = document.createElement('a');
      back.className = 'fed-page-back';
      back.href = parentNavHref(app);
      back.setAttribute(
        'aria-label',
        'Back to ' + (hubParent ? hubParent.config.label : app.label)
      );

      var icon = document.createElement('i');
      icon.className = 'material-icons';
      icon.textContent = 'arrow_back';
      back.appendChild(icon);
      header.appendChild(back);
    }
    header.appendChild(bodyWrap);
  }

  function mountAppContextNav(app) {
    if (!app || !app.sections || isWorkspacePage(app)) return;
    if (document.querySelector('.fed-app-nav')) return;

    var main = document.querySelector('main.main-content, main.container.main-wrap, main.container, main.portal-main, main.fed-shell-main');
    if (!main) return;

    var nav = document.createElement('div');
    nav.className = 'fed-app-nav';
    nav.setAttribute('role', 'navigation');
    nav.setAttribute('aria-label', app.label + ' navigation');

    for (var s = 0; s < app.sections.length; s++) {
      var section = app.sections[s];
      var visibleItems = [];
      for (var i = 0; i < section.items.length; i++) {
        if (itemVisible(section.items[i])) visibleItems.push(section.items[i]);
      }
      if (!visibleItems.length) continue;

      var sec = document.createElement('div');
      sec.className = 'fed-app-nav-section';
      sec.innerHTML = '<h2 class="fed-app-nav-section-title">' + section.title + '</h2>';

      var card = document.createElement('div');
      card.className = 'fed-app-nav-card';
      var ul = document.createElement('ul');
      ul.className = 'fed-app-nav-list browser-default';

      for (var v = 0; v < visibleItems.length; v++) {
        var item = visibleItems[v];
        var li = document.createElement('li');
        if (activeItemHref(app, item)) li.className = 'is-active';
        var a = document.createElement('a');
        a.href = cleanPublicHref(appUrl(app, item.href));
        if (item.external) {
          a.target = '_blank';
          a.rel = 'noopener';
        }
        a.innerHTML = buildNavLinkInner(app, item, { external: item.external });
        li.appendChild(a);
        ul.appendChild(li);
      }
      card.appendChild(ul);
      sec.appendChild(card);
      nav.appendChild(sec);
    }

    var header = main.querySelector('.page-header, .fed-shell-page-header');
    if (header && header.nextSibling) {
      main.insertBefore(nav, header.nextSibling);
    } else {
      main.insertBefore(nav, main.firstChild);
    }
  }

  function isTopLevelHub(app, hubId) {
    if (!app || !hubId) return false;
    var appNav = config.appNav || [];
    for (var i = 0; i < appNav.length; i++) {
      if (appNav[i].appId === app.id && appNav[i].hubId === hubId) return true;
    }
    return false;
  }

  function renderHub(app, hubPage) {
    var hub = hubPage.config;
    var main = findWorkspaceMain();
    if (!main) {
      main = document.createElement('main');
      main.className = 'fed-shell-main';
      main.setAttribute('data-federation-hub', hubPage.id);
      document.body.appendChild(main);
    } else {
      main.classList.add('fed-shell-main');
      main.setAttribute('data-federation-hub', hubPage.id);
    }

    var root = main.querySelector('.fed-shell-page');
    // Server-rendered hub HTML — do not replace (avoids a content flash).
    if (root && root.getAttribute('data-fed-ssr') === '1') {
      return;
    }
    if (!root) {
      root = document.createElement('div');
      root.className = 'fed-shell-page';
      main.innerHTML = '';
      main.appendChild(root);
    }

    var topLevel = hub.topLevel === true || isTopLevelHub(app, hubPage.id);
    var html;
    if (topLevel) {
      html =
        '<header class="fed-shell-page-header">' +
        '<h1 class="fed-shell-page-title">' +
        hub.label +
        '</h1>' +
        '<p class="fed-shell-page-subtitle">' +
        (hub.description || '') +
        '</p></header>';
    } else {
      var homeHref = cleanPublicHref(sectionHomeHref(app));
      var parentLabel = hub.parentLabel || app.label;
      html =
        '<header class="fed-shell-page-header fed-page-header fed-page-header-enhanced fed-hub-page-header">' +
        '<a class="fed-page-back" href="' +
        homeHref +
        '" aria-label="Back to ' +
        parentLabel +
        '"><i class="material-icons">arrow_back</i></a>' +
        '<div class="fed-page-header-body">' +
        '<p class="fed-hub-breadcrumb"><a href="' +
        homeHref +
        '">' +
        parentLabel +
        '</a><span class="fed-hub-breadcrumb-sep"> / </span><span>' +
        hub.label +
        '</span></p>' +
        '<h1 class="fed-shell-page-title">' +
        hub.label +
        '</h1>' +
        '<p class="fed-shell-page-subtitle">' +
        (hub.description || '') +
        '</p></div></header>';
    }
    html +=
      '<div class="fed-app-nav fed-workspace-nav" role="navigation" aria-label="' +
      hub.label +
      '">' +
      buildAppNavSections(
        { urlPrefix: app.urlPrefix, mount: app.mount, sections: hub.sections },
        { markActive: false }
      ) +
      '</div>';
    root.innerHTML = html;
  }

  function renderWorkspace(app) {
    if (isStaticWorkspaceContent()) return;
    var main = findWorkspaceMain();
    if (!main) {
      main = document.createElement('main');
      main.className = 'fed-shell-main';
      main.setAttribute('data-federation-workspace', 'true');
      document.body.appendChild(main);
    } else {
      main.classList.add('fed-shell-main');
      main.setAttribute('data-federation-workspace', 'true');
    }

    if (app && app.id === 'central') {
      document.body.classList.add('fed-central-workspace');
      if (document.body.getAttribute('data-federation-app') !== 'central') {
        document.body.setAttribute('data-federation-app', 'central');
      }
      hideLegacyPortalChrome();
    }

    var root = main.querySelector('.fed-shell-page');
    if (!root) {
      root = document.createElement('div');
      root.className = 'fed-shell-page';
      main.innerHTML = '';
      main.appendChild(root);
    }

    var html = '<header class="fed-shell-page-header"><h1 class="fed-shell-page-title">' + app.label + '</h1>';
    html += '<p class="fed-shell-page-subtitle">' + (app.description || '') + '</p></header>';
    html +=
      '<div class="fed-app-nav fed-workspace-nav" role="navigation" aria-label="' +
      app.label +
      ' workspace">' +
      buildAppNavSections(app, { markActive: false }) +
      '</div>';
    root.innerHTML = html;
  }

  function loadQgisFlags() {
    var app = appConfig('qgis-publish');
    if (!app || typeof global.publishURL !== 'function') return Promise.resolve();
    return fetch(global.publishURL('/config.json'), { headers: { Accept: 'application/json' } })
      .then(function (res) {
        return res.ok ? res.json() : {};
      })
      .then(function (data) {
        global.FEDERATION_SHELL_FLAGS = {
          enable_qgis: data.enable_qgis !== false,
          enable_raster: data.enable_raster !== false,
        };
      })
      .catch(function () {
        global.FEDERATION_SHELL_FLAGS = { enable_qgis: true, enable_raster: true };
      });
  }

  function init() {
    ensureRobotoFont();
    if (isShellDisabled()) {
      clearShellChrome();
      return;
    }
    ensureStylesheet();
    var appId = detectAppId();
    if (!appId && isCentralServicesPath()) {
      appId = 'central';
    }
    var app = appConfig(appId);

    if (isNoSidebarPage()) {
      clearSidebarChrome();
      document.documentElement.classList.add('fed-shell-ready');
      if (document.body) document.body.classList.add('fed-shell-body');
    } else {
      mountSidebar(appId);
    }
    var hubPage = app && detectHubPage(app);
    if (hubPage) {
      renderHub(app, hubPage);
    } else if (app && isWorkspacePage(app)) {
      renderWorkspace(app);
    } else {
      if (app) enhanceAppPageHeader(app);
      if (app && app.showContextNav) mountAppContextNav(app);
    }

    if (appId === 'qgis-publish') {
      loadQgisFlags().then(function () {
        if (app && isWorkspacePage(app)) {
          renderWorkspace(app);
        }
      });
    }
  }

  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', init);
  } else {
    init();
  }

  global.FederationShell = {
    detectAppId: detectAppId,
    appConfig: appConfig,
    appUrl: appUrl,
    renderWorkspace: renderWorkspace,
    mountSidebar: mountSidebar,
    mountAppContextNav: mountAppContextNav,
    enhanceAppPageHeader: enhanceAppPageHeader,
  };
})(typeof window !== 'undefined' ? window : this);
