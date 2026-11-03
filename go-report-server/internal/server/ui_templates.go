package server

// Portal catalog markup mirrors reports/index.php (Materialize + portal.css).
// Hubs chrome mirrors hubs/*.php (shell-bootstrap + shell.js fills main).
// Viewer markup/CSS/JS mirrors reports/view.php (map-style.js from PHP assets).

const hubHTML = `<!DOCTYPE html>
<html lang="en" class="fed-shell-ready">
<head>
    <meta charset="utf-8">
    <meta name="viewport" content="width=device-width, initial-scale=1">
    <title>{{.Title}} - {{.ProductName}}</title>
    <style id="fed-shell-critical">{{.CriticalCSS}}</style>
    <link rel="stylesheet" href="https://fonts.googleapis.com/css2?family=Roboto:wght@400;500&display=swap">
    <link rel="stylesheet" href="{{.AssetsURL}}/shell.css?v=20260727h" id="fed-shell-css">
    <link href="https://fonts.googleapis.com/icon?family=Material+Icons&display=block" rel="stylesheet">
    <script>
      window.FEDERATION_SHELL_MOUNT = {{.ShellMount}};
      window.BASE_PATH = {{.ShellMount}};
      window.FEDERATION_PHP_MOUNT = {{.PHPMount}};
      window.FEDERATION_SHELL_ASSETS = {{.AssetsBase}};
    </script>
    <script src="{{.AssetsURL}}/shell-bootstrap.js?v=20261011b" defer></script>
</head>
<body data-federation-app="jasper" data-federation-hub="{{.HubID}}" class="fed-shell-body">
<aside id="fed-shell-sidebar" class="fed-shell-sidebar" aria-label="Application navigation">
    {{if .ShowSidebarBrand}}<a class="fed-shell-sidebar-brand" href="{{.BrandHome}}"><i class="material-icons">assessment</i><span>{{.ProductName}}</span></a>{{end}}
    <div class="fed-shell-nav-scroll" data-fed-shell-nav-mount data-fed-ssr="1">{{.SidebarNav}}</div>
</aside>
<button type="button" class="fed-shell-menu-btn" aria-label="Open menu" hidden><i class="material-icons">menu</i></button>
<div class="fed-shell-backdrop"></div>
<main class="fed-shell-main" data-federation-hub="{{.HubID}}">{{.HubBody}}</main>
</body>
</html>`

const workspaceHTML = `<!DOCTYPE html>
<html lang="en" class="fed-shell-ready">
<head>
    <meta charset="utf-8">
    <meta name="viewport" content="width=device-width, initial-scale=1">
    <title>{{.ProductName}}</title>
    <style id="fed-shell-critical">{{.CriticalCSS}}</style>
    <link rel="stylesheet" href="https://fonts.googleapis.com/css2?family=Roboto:wght@400;500&display=swap">
    <link rel="stylesheet" href="{{.AssetsURL}}/shell.css?v=20260727h" id="fed-shell-css">
    <link href="https://fonts.googleapis.com/icon?family=Material+Icons&display=block" rel="stylesheet">
    <script>
      window.FEDERATION_SHELL_MOUNT = {{.ShellMount}};
      window.BASE_PATH = {{.ShellMount}};
      window.FEDERATION_PHP_MOUNT = {{.PHPMount}};
      window.FEDERATION_SHELL_ASSETS = {{.AssetsBase}};
    </script>
    <script src="{{.AssetsURL}}/shell-bootstrap.js?v=20261011b" defer></script>
</head>
<body data-federation-app="jasper" class="fed-shell-body">
<aside id="fed-shell-sidebar" class="fed-shell-sidebar" aria-label="Application navigation">
    {{if .ShowSidebarBrand}}<a class="fed-shell-sidebar-brand" href="{{.BrandHome}}"><i class="material-icons">assessment</i><span>{{.ProductName}}</span></a>{{end}}
    <div class="fed-shell-nav-scroll" data-fed-shell-nav-mount data-fed-ssr="1">{{.SidebarNav}}</div>
</aside>
<button type="button" class="fed-shell-menu-btn" aria-label="Open menu" hidden><i class="material-icons">menu</i></button>
<div class="fed-shell-backdrop"></div>
<main class="fed-shell-main" data-federation-workspace></main>
</body>
</html>`

const catalogHTML = `<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>AcuGIS Report Server - {{.ProductName}}</title>
  <link href="https://cdnjs.cloudflare.com/ajax/libs/materialize/1.0.0/css/materialize.min.css" rel="stylesheet">
  <link href="https://fonts.googleapis.com/icon?family=Material+Icons" rel="stylesheet">
  <link href="https://fonts.googleapis.com/css2?family=Roboto:wght@300;400;500;700&display=swap" rel="stylesheet">
  <link href="{{.PortalCSS}}" rel="stylesheet">
</head>
<body data-federation-no-shell class="portal-home">
  <nav class="topnav" role="navigation">
    <div class="nav-wrapper">
      <a href="{{.HomeURL}}" class="brand-logo">
        <i class="material-icons brand-mark">public</i>
        <span class="brand-name">AcuGIS Report Server</span>
      </a>

      <ul class="right hide-on-med-and-down topnav-actions">
        {{if .IsPlatformAdmin}}
        <li>
          <a href="{{.AdminURL}}" class="topnav-link">
            <i class="material-icons left">admin_panel_settings</i><span class="link-label">Administration</span>
          </a>
        </li>
        {{end}}
        {{if .Username}}
        <li>
          <a href="{{.AccountURL}}" class="topnav-link">
            <i class="material-icons left">manage_accounts</i><span class="link-label">Account</span>
          </a>
        </li>
        <li>
          <span class="topnav-link topnav-username" title="{{.Username}}">
            Welcome {{.Username}}
          </span>
        </li>
        {{end}}
        <li>
          <a href="{{.LogoutURL}}" class="topnav-link">
            <i class="material-icons left">logout</i><span class="link-label">Logout</span>
          </a>
        </li>
      </ul>
    </div>
  </nav>

  <main class="portal-main portal-content">
    <div class="portal-hero-band">
      <header class="portal-hero">
        <div class="hero-text">
         <h1 class="hero-title">
    Welcome {{.Username}}
</h1>          <p class="hero-subtitle"></p>
        </div>
      </header>

      <div class="portal-search-section" role="search">
        <div class="portal-search-wrap">
          <label class="sr-only" for="portal-search">Search portal resources</label>
          <i class="material-icons portal-search-icon" aria-hidden="true">search</i>&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;
          <input
            id="portal-search"
            class="portal-search-input"
            type="search"
            autocomplete="off"
            placeholder="Search reports..."
          >
        </div>
      </div>
    </div>

    <section class="portal-section portal-explore" aria-labelledby="explore-heading">
      <div class="section-head section-head-row">
        <h2 id="explore-heading" class="section-title">Explore</h2>
        {{if .IsAdminOrEditor}}
        <div class="section-head-actions">
          <a href="{{.PublishURL}}"><i class="material-icons" style="font-size:18px;vertical-align:middle;">add</i> Publish report</a>
        </div>
        {{end}}
      </div>

      <div class="portal-grid" id="portal-card-grid" aria-live="polite">
        {{if not .Cards}}
          <article class="resource-card resource-card--state">
            <div class="resource-card-content resource-card-content--center">
              <div class="portal-state-icon"><i class="material-icons">assessment</i></div>
              <h3 class="resource-card-title">No reports published yet</h3>
              <p class="resource-card-desc">
                {{if .IsAdminOrEditor}}
                  Publish a Jasper report to see it appear here.
                {{else}}
                  Check back once reports have been published for your account.
                {{end}}
              </p>
              {{if .IsAdminOrEditor}}
                <a href="{{.PublishURL}}" class="resource-open-btn" style="margin-top:12px;">Publish</a>
              {{end}}
            </div>
          </article>
        {{else}}
          {{range .Cards}}
            <article class="resource-card card-type-report" data-search="{{.SearchBlob}}">
              {{if .ThumbURL}}
                <div class="resource-preview {{.PreviewClass}}">
                  <img src="{{.ThumbURL}}" alt="">
                </div>
              {{else}}
                <div class="resource-preview {{.PreviewClass}}" aria-hidden="true">
                  <span class="resource-preview-label">{{.PreviewLabel}}</span>
                </div>
              {{end}}
              <div class="resource-card-content">
                <h3 class="resource-card-title">{{.Name}}</h3>
                <p class="resource-card-desc">{{.Description}}</p>
                <div class="resource-card-footer">
                  <span class="resource-type-badge {{.BadgeClass}}">{{.TypeLabel}}</span>
                  <div class="resource-card-actions">
                    {{if .CanEdit}}
                      <a class="resource-icon-btn" href="{{.EditURL}}" title="Edit" aria-label="Edit {{.Name}}">
                        <i class="material-icons">edit</i>
                      </a>
                    {{end}}
                    {{if .CanAdmin}}
                      <a class="resource-icon-btn resource-icon-btn--danger" href="{{.DeleteURL}}" title="Delete" aria-label="Delete {{.Name}}" onclick="return confirm('Delete this report?');">
                        <i class="material-icons">delete</i>
                      </a>
                    {{end}}
                    <a href="{{.ViewerURL}}" class="resource-open-btn" aria-label="Open {{.Name}}">Open</a>
                  </div>
                </div>
              </div>
            </article>
          {{end}}
        {{end}}
      </div>
    </section>
  </main>

  <footer class="portal-footer">
    <span>AcuGIS Portal &middot; Unified Spatial Workspace</span>
    <span class="footer-spacer"></span>
    <span class="footer-version">v0.1</span>
  </footer>

  <script src="https://cdnjs.cloudflare.com/ajax/libs/materialize/1.0.0/js/materialize.min.js"></script>
  <script>
  (function () {
    var search = document.getElementById('portal-search');
    var grid = document.getElementById('portal-card-grid');
    if (!search || !grid) return;

    search.addEventListener('input', function () {
      var q = (search.value || '').trim().toLowerCase();
      var cards = grid.querySelectorAll('.resource-card[data-search]');
      var visible = 0;
      cards.forEach(function (card) {
        var hay = card.getAttribute('data-search') || '';
        var show = !q || hay.indexOf(q) !== -1;
        card.classList.toggle('is-hidden', !show);
        if (show) visible += 1;
      });

      var empty = grid.querySelector('.resource-card--filter-empty');
      if (q && visible === 0 && cards.length) {
        if (!empty) {
          empty = document.createElement('article');
          empty.className = 'resource-card resource-card--state resource-card--filter-empty';
          empty.innerHTML =
            '<div class="resource-card-content resource-card-content--center">' +
              '<div class="portal-state-icon"><i class="material-icons">search_off</i></div>' +
              '<h3 class="resource-card-title">No matching cards</h3>' +
              '<p class="resource-card-desc">Try a different search term.</p>' +
            '</div>';
          grid.appendChild(empty);
        }
      } else if (empty) {
        empty.remove();
      }
    });
  })();
  </script>
</body>
</html>`
