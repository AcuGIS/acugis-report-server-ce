/**
 * Loads shell JS from /shared/ui (vendored with AcuGIS Report Server).
 * Override: window.FEDERATION_SHELL_ASSETS
 *
 * Critical CSS is injected immediately when this file runs (sync <script> in
 * <head>) so the hamburger never paints before layout CSS — even if the page
 * HTML was built from an older binary without inline critical styles.
 */
(function (w) {
  'use strict';

  function detectMount() {
    var p = (w.location && w.location.pathname) || '';
    if (p.indexOf('/acugis-report-server') === 0) return '/acugis-report-server';
    if (p.indexOf('/jasper-report-services') === 0) return '/jasper-report-services';
    // Go root deploy sets BASE_PATH / FEDERATION_SHELL_MOUNT to "" — keep it.
    if (Object.prototype.hasOwnProperty.call(w, 'BASE_PATH')) {
      return String(w.BASE_PATH || '').replace(/\/$/, '');
    }
    return '/acugis-report-server';
  }

  // Only fill mount when the page did not set it (including explicit "").
  if (!Object.prototype.hasOwnProperty.call(w, 'FEDERATION_SHELL_MOUNT')) {
    w.FEDERATION_SHELL_MOUNT = detectMount();
  } else {
    w.FEDERATION_SHELL_MOUNT = String(w.FEDERATION_SHELL_MOUNT || '').replace(/\/$/, '');
  }

  function resolveAssetsBase() {
    if (w.FEDERATION_SHELL_ASSETS) {
      return String(w.FEDERATION_SHELL_ASSETS).replace(/\/$/, '');
    }
    return '/shared/ui';
  }

  var assetsBase = resolveAssetsBase();
  var version = '20261011b';

  function shouldLoadShell() {
    if (typeof document !== 'undefined' && document.body && document.body.hasAttribute('data-federation-no-shell')) {
      return false;
    }
    return true;
  }

  var cssHref = assetsBase + '/shell.css?v=' + version;

  function headEl() {
    return document.head || document.getElementsByTagName('head')[0] || document.documentElement;
  }

  function ensureCriticalCss() {
    if (document.getElementById('fed-shell-critical')) return;
    var style = document.createElement('style');
    style.id = 'fed-shell-critical';
    style.textContent =
      '.fed-shell-menu-btn,[hidden].fed-shell-menu-btn{display:none!important}' +
      '.fed-shell-backdrop{display:none!important}' +
      'body.fed-shell-body{margin:0;padding-left:var(--fed-shell-sidebar-width,250px);box-sizing:border-box;background:#f8f9fa}' +
      '.fed-shell-sidebar{position:fixed;top:0;left:0;bottom:0;width:var(--fed-shell-sidebar-width,250px);' +
      'z-index:1000;overflow:hidden;background:#f0f4f9}' +
      '@media (max-width:992px){' +
      'body.fed-shell-body{padding-left:0}' +
      '.fed-shell-sidebar{transform:translateX(-100%)}' +
      'body.fed-shell-open .fed-shell-sidebar{transform:translateX(0)}' +
      'body.fed-shell-open .fed-shell-backdrop{display:block!important}' +
      '.fed-shell-menu-btn{display:flex!important;align-items:center;justify-content:center;' +
      'position:fixed;top:12px;left:12px;z-index:1002;width:44px;height:44px;border:none;' +
      'border-radius:50%;background:#fff;cursor:pointer}' +
      '}';
    headEl().appendChild(style);
  }

  function ensureStylesheet() {
    if (document.getElementById('fed-shell-css')) return;
    var link = document.createElement('link');
    link.id = 'fed-shell-css';
    link.rel = 'stylesheet';
    link.href = cssHref;
    headEl().appendChild(link);
  }

  function loadScript(name, done) {
    var src = assetsBase + '/' + name + '?v=' + version;
    var existing = document.querySelector('script[data-fed-shell="' + name + '"]');
    if (existing) {
      if (existing.getAttribute('data-fed-shell-loaded') === '1') {
        done();
        return;
      }
      existing.addEventListener('load', done);
      existing.addEventListener('error', done);
      return;
    }
    var s = document.createElement('script');
    s.src = src;
    s.async = false;
    s.setAttribute('data-fed-shell', name);
    s.onload = function () {
      s.setAttribute('data-fed-shell-loaded', '1');
      done();
    };
    s.onerror = done;
    headEl().appendChild(s);
  }

  // Run while this blocking <script> is still in <head>, before <body> parses.
  if (shouldLoadShell()) {
    ensureCriticalCss();
    ensureStylesheet();
  }

  function applyEditionPayload(data) {
    var caps = (data && data.capabilities) || data || {};
    w.FEDERATION_EDITION_CAPS = {
      edition: (data && data.edition) || caps.edition || 'community',
      features: caps.features || {},
    };
    var flags = w.FEDERATION_SHELL_FLAGS || {};
    flags.reportDesigner = !!(data && data.report_designer);
    w.FEDERATION_SHELL_FLAGS = flags;
  }

  function editionURL() {
    var mount = String(w.FEDERATION_SHELL_MOUNT || '').replace(/\/$/, '');
    return mount + '/api/v1/edition';
  }

  function loadEditionCaps(done) {
    if (w.FEDERATION_EDITION_CAPS && w.FEDERATION_EDITION_CAPS.features) {
      done();
      return;
    }
    var finished = false;
    function finish(payload) {
      if (finished) return;
      finished = true;
      applyEditionPayload(payload);
      done();
    }
    var timer = w.setTimeout(function () {
      finish({ edition: 'community', capabilities: { features: {} } });
    }, 2500);
    var fetchFn = typeof w.fetch === 'function' ? w.fetch : null;
    if (!fetchFn) {
      w.clearTimeout(timer);
      finish({ edition: 'community', capabilities: { features: {} } });
      return;
    }
    fetchFn(editionURL(), { credentials: 'same-origin', headers: { Accept: 'application/json' } })
      .then(function (res) {
        return res.ok ? res.json() : {};
      })
      .then(function (data) {
        w.clearTimeout(timer);
        finish(data);
      })
      .catch(function () {
        w.clearTimeout(timer);
        finish({ edition: 'community', capabilities: { features: {} } });
      });
  }

  function start() {
    if (!shouldLoadShell()) return;
    ensureCriticalCss();
    ensureStylesheet();
    loadEditionCaps(function () {
      loadScript('shell-config.js', function () {
        loadScript('shell.js', function () {});
      });
    });
  }

  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', start);
  } else {
    start();
  }
})(typeof window !== 'undefined' ? window : this);
