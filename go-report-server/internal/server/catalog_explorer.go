package server

import (
	"html/template"
)

// catalogExplorerHTML is the file-explorer style content catalog.
const catalogExplorerHTML = `<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>AcuGIS Report Server - {{.ProductName}}</title>
  <link href="https://cdnjs.cloudflare.com/ajax/libs/materialize/1.0.0/css/materialize.min.css" rel="stylesheet">
  <link href="https://fonts.googleapis.com/icon?family=Material+Icons" rel="stylesheet">
  <link href="https://fonts.googleapis.com/css2?family=Roboto:wght@300;400;500;700&display=swap" rel="stylesheet">
  <link href="{{.PortalCSS}}" rel="stylesheet">
  <style>
    .fe-layout{display:grid;grid-template-columns:260px 1fr;gap:0;min-height:calc(100vh - var(--topnav-height) - 80px);background:var(--acu-bg-alt);}
    .fe-tree{background:var(--acu-surface);border-right:1px solid var(--acu-border);padding:12px 8px;overflow:auto;}
    .fe-tree-title{font-size:12px;font-weight:500;color:var(--acu-text-muted);text-transform:uppercase;letter-spacing:.04em;padding:4px 10px 10px;}
    .fe-tree ul{list-style:none;margin:0;padding:0;}
    .fe-tree li{margin:0;}
    .fe-tree-node{display:flex;align-items:center;gap:4px;padding:4px 6px;border-radius:8px;cursor:pointer;user-select:none;color:var(--acu-text);}
    .fe-tree-node:hover{background:var(--acu-accent-soft);}
    .fe-tree-node.is-active{background:var(--acu-accent-soft);color:var(--acu-accent-strong);font-weight:500;}
    .fe-tree-toggle{width:22px;height:22px;border:0;background:transparent;cursor:pointer;color:var(--acu-text-muted);display:inline-flex;align-items:center;justify-content:center;padding:0;}
    .fe-tree-toggle .material-icons{font-size:18px;}
    .fe-tree-label{display:inline-flex;align-items:center;gap:6px;flex:1;min-width:0;}
    .fe-tree-label .material-icons{font-size:18px;color:var(--acu-accent);}
    .fe-tree-label span{overflow:hidden;text-overflow:ellipsis;white-space:nowrap;}
    .fe-tree-children{padding-left:16px;}
    .fe-tree-children.is-collapsed{display:none;}
    .fe-main{display:flex;flex-direction:column;min-width:0;background:var(--acu-bg);}
    .fe-toolbar{display:flex;align-items:center;gap:12px;flex-wrap:wrap;padding:14px 20px;border-bottom:1px solid var(--acu-border);background:var(--acu-surface);}
    .fe-breadcrumbs{display:flex;align-items:center;gap:4px;flex-wrap:wrap;font-size:14px;color:var(--acu-text-muted);background:transparent;box-shadow:none;height:auto;line-height:1.4;padding:0;margin:0;}
    .fe-breadcrumbs a{color:var(--acu-accent-strong);text-decoration:none;}
    .fe-breadcrumbs a:hover{text-decoration:underline;}
    .fe-breadcrumbs .sep{opacity:.5;}
    .fe-toolbar-actions{margin-left:auto;display:flex;gap:8px;align-items:center;}
    .fe-btn{display:inline-flex;align-items:center;gap:6px;border:1px solid var(--acu-border);background:#fff;border-radius:8px;padding:7px 12px;font-size:13px;cursor:pointer;color:var(--acu-text);text-decoration:none;}
    .fe-btn:hover{border-color:var(--acu-accent);color:var(--acu-accent-strong);}
    .fe-btn .material-icons{font-size:18px;}
    .fe-btn-primary{background:var(--acu-accent);border-color:var(--acu-accent);color:#fff;}
    .fe-btn-primary:hover{background:var(--acu-accent-strong);color:#fff;}
    .fe-search-wrap{position:relative;flex:1;min-width:180px;max-width:360px;}
    .fe-search-wrap .material-icons{position:absolute;left:10px;top:50%;transform:translateY(-50%);font-size:18px;color:var(--acu-text-subtle);}
    .fe-search-input{width:100%!important;height:36px!important;margin:0!important;padding-left:36px!important;border:1px solid var(--acu-border)!important;border-radius:8px!important;box-sizing:border-box;}
    .fe-body{padding:16px 20px 32px;flex:1;}
    .fe-list{display:flex;flex-direction:column;gap:4px;}
    .fe-row{display:grid;grid-template-columns:40px 1fr 160px 120px 44px;align-items:center;gap:8px;padding:10px 12px;border-radius:10px;border:1px solid transparent;background:var(--acu-surface);cursor:default;user-select:none;}
    .fe-row:hover,.fe-row.is-drop-target{border-color:var(--acu-accent);background:#f7fffe;}
    .fe-row.is-dragging{opacity:.55;}
    .fe-row-icon .material-icons{font-size:28px;color:var(--acu-accent);}
    .fe-row-icon.is-report .material-icons{color:#5c6bc0;}
    .fe-row-icon.is-map .material-icons{color:#00897b;}
    .fe-row-icon.is-resource .material-icons{color:#ef6c00;}
    .fe-row-icon.is-view .material-icons{color:#1565c0;}
    .fe-row-icon.is-dashboard .material-icons{color:#00897b;}
    .fe-row-icon.is-image{width:40px;height:40px;border-radius:8px;overflow:hidden;background:#f1f5f9;display:flex;align-items:center;justify-content:center;}
    .fe-row-icon.is-image img{width:100%;height:100%;object-fit:cover;display:block;}
    .fe-row-icon.is-image .material-icons{color:#00897b;font-size:28px;}
    .fe-row-name{font-weight:500;color:var(--acu-text);overflow:hidden;text-overflow:ellipsis;white-space:nowrap;}
    .fe-row-desc{font-size:12px;color:var(--acu-text-muted);overflow:hidden;text-overflow:ellipsis;white-space:nowrap;}
    .fe-row-path{font-size:12px;color:var(--acu-text-subtle);overflow:hidden;text-overflow:ellipsis;white-space:nowrap;}
    .fe-row-type{font-size:12px;color:var(--acu-text-muted);}
    .fe-row-menu{border:0;background:transparent;cursor:pointer;border-radius:6px;width:32px;height:32px;color:var(--acu-text-muted);}
    .fe-row-menu:hover{background:var(--acu-accent-soft);color:var(--acu-accent-strong);}
    .fe-new-wrap{position:relative;display:inline-flex;}
    .fe-new-menu{display:none;position:absolute;right:0;top:calc(100% + 4px);min-width:180px;background:#fff;border:1px solid var(--acu-border);border-radius:10px;box-shadow:var(--acu-shadow-md);padding:6px 0;z-index:50;}
    .fe-new-menu.is-open{display:block;}
    .fe-new-menu button,.fe-new-menu a{display:flex;width:100%;align-items:center;gap:10px;border:0;background:transparent;padding:8px 14px;font-size:13px;cursor:pointer;color:var(--acu-text);text-align:left;text-decoration:none;box-sizing:border-box;}
    .fe-new-menu button:hover,.fe-new-menu a:hover{background:var(--acu-accent-soft);}
    .fe-new-menu .material-icons{font-size:18px;}
    .fe-wizard-step{margin-bottom:14px;}
    .fe-wizard-step label{display:block;font-size:13px;font-weight:500;margin-bottom:6px;color:var(--acu-text);}
    .fe-wizard-radios label{display:flex;align-items:center;gap:8px;font-weight:400;margin:6px 0;cursor:pointer;}
    .fe-wizard-tabs{display:flex;gap:8px;margin-bottom:10px;}
    .fe-wizard-tabs button{flex:1;}
    .fe-wizard-tabs button.is-active{background:var(--acu-accent);border-color:var(--acu-accent);color:#fff;}
    .fe-dropzone{border:2px dashed var(--acu-border);border-radius:10px;padding:28px 16px;text-align:center;color:var(--acu-text-muted);background:#fafbfc;cursor:pointer;transition:border-color .15s,background .15s;}
    .fe-dropzone.is-dragover{border-color:var(--acu-accent);background:var(--acu-accent-soft);color:var(--acu-accent-strong);}
    .fe-dropzone .material-icons{font-size:36px;display:block;margin:0 auto 8px;opacity:.7;}
    .fe-dropzone-file{font-size:13px;margin-top:8px;color:var(--acu-text);}
    .fe-img-preview{max-width:100%;max-height:360px;display:block;margin:0 auto 16px;border-radius:8px;background:
      linear-gradient(45deg,#eee 25%,transparent 25%),linear-gradient(-45deg,#eee 25%,transparent 25%),
      linear-gradient(45deg,transparent 75%,#eee 75%),linear-gradient(-45deg,transparent 75%,#eee 75%);
      background-size:16px 16px;background-position:0 0,0 8px,8px -8px,-8px 0;}
    .fe-img-meta{display:grid;grid-template-columns:120px 1fr;gap:6px 12px;font-size:13px;margin-bottom:12px;}
    .fe-img-meta dt{color:var(--acu-text-muted);margin:0;}
    .fe-img-meta dd{margin:0;color:var(--acu-text);word-break:break-all;}
    .fe-img-actions{display:flex;flex-wrap:wrap;gap:8px;margin-top:8px;}
    .fe-modal.fe-modal-wide{width:min(640px,94vw);}
    .fe-empty{text-align:center;padding:48px 16px;color:var(--acu-text-muted);}
    .fe-empty .material-icons{font-size:48px;opacity:.4;}
    .fe-ctx{position:fixed;z-index:2000;min-width:180px;background:#fff;border:1px solid var(--acu-border);border-radius:10px;box-shadow:var(--acu-shadow-md);padding:6px 0;display:none;}
    .fe-ctx button{display:flex;width:100%;align-items:center;gap:10px;border:0;background:transparent;padding:8px 14px;font-size:13px;cursor:pointer;color:var(--acu-text);text-align:left;}
    .fe-ctx button:hover{background:var(--acu-accent-soft);}
    .fe-ctx button.danger{color:#c62828;}
    .fe-ctx .material-icons{font-size:18px;}
    .fe-modal-backdrop{position:fixed;inset:0;background:rgba(15,23,42,.35);z-index:3000;display:none;align-items:center;justify-content:center;}
    .fe-modal{background:#fff;border-radius:12px;padding:20px;width:min(420px,92vw);box-shadow:var(--acu-shadow-md);}
    .fe-modal h3{margin:0 0 12px;font-size:18px;}
    .fe-modal input,.fe-modal textarea,.fe-modal select{width:100%;box-sizing:border-box;}
    .fe-modal textarea{min-height:140px;font-family:ui-monospace,Consolas,monospace;font-size:12px;}
    .fe-modal-actions{display:flex;justify-content:flex-end;gap:8px;margin-top:16px;}
    .portal-hero-band{display:none;}
    @media (max-width:900px){.fe-layout{grid-template-columns:1fr;}.fe-tree{max-height:220px;border-right:0;border-bottom:1px solid var(--acu-border);}.fe-row{grid-template-columns:36px 1fr 40px;}.fe-row-path,.fe-row-type{display:none;}}
  </style>
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
        <li><span class="topnav-link topnav-username" title="{{.Username}}">Welcome {{.Username}}</span></li>
        {{end}}
        <li>
          <a href="{{.LogoutURL}}" class="topnav-link">
            <i class="material-icons left">logout</i><span class="link-label">Logout</span>
          </a>
        </li>
      </ul>
    </div>
  </nav>

  <main class="portal-main portal-content" style="padding:0;">
    <div class="fe-layout" id="fe-root"
         data-api-base="{{.APIBase}}"
         data-can-edit="{{if .IsAdminOrEditor}}1{{else}}0{{end}}"
         data-publish-url="{{.PublishURL}}"
         data-designer-create="{{.DesignerCreateURL}}"
         data-designer-import="{{.DesignerImportURL}}"
         data-explorer-url="{{.ExplorerURL}}"
         data-allow-recursive="{{if .AllowRecursiveDelete}}1{{else}}0{{end}}">
      <aside class="fe-tree" aria-label="Folder navigation">
        <div class="fe-tree-title">Folders</div>
        <div id="fe-tree-mount"></div>
      </aside>
      <section class="fe-main">
        <div class="fe-toolbar">
          <div class="fe-breadcrumbs" id="fe-breadcrumbs" aria-label="Breadcrumb"></div>
          <div class="fe-search-wrap">
            <i class="material-icons" aria-hidden="true">search</i>
            <input id="fe-search" class="fe-search-input" type="search" placeholder="Search all folders…" autocomplete="off">
          </div>
          <div class="fe-toolbar-actions">
            {{if .IsAdminOrEditor}}
            <div class="fe-new-wrap">
              <button type="button" class="fe-btn fe-btn-primary" id="fe-new-menu-btn"><i class="material-icons">add</i> New</button>
              <div class="fe-new-menu" id="fe-new-menu" role="menu">
                <button type="button" data-new="folder"><i class="material-icons">create_new_folder</i> Folder</button>
                <a href="{{.PublishURL}}"><i class="material-icons">assessment</i> Report</a>
                {{if .DesignerCreateURL}}<a href="{{.DesignerCreateURL}}"><i class="material-icons">design_services</i> Create Designer Report</a>{{end}}
                {{if .DesignerImportURL}}<a href="{{.DesignerImportURL}}"><i class="material-icons">unarchive</i> Import Designer Project</a>{{end}}
                <button type="button" data-new="view"><i class="material-icons">travel_explore</i> Saved View</button>
                <button type="button" data-new="dashboard"><i class="material-icons">dashboard</i> Dashboard</button>
                <button type="button" data-new="resource"><i class="material-icons">vpn_key</i> Resource</button>
              </div>
            </div>
            {{end}}
          </div>
        </div>
        <div class="fe-body">
          <div class="fe-list" id="fe-list" aria-live="polite"></div>
        </div>
      </section>
    </div>
  </main>

  <div class="fe-ctx" id="fe-ctx" role="menu"></div>
  <div class="fe-modal-backdrop" id="fe-modal-backdrop">
    <div class="fe-modal" id="fe-modal" role="dialog" aria-modal="true">
      <h3 id="fe-modal-title">Dialog</h3>
      <div id="fe-modal-body"></div>
      <div class="fe-modal-actions">
        <button type="button" class="fe-btn" id="fe-modal-cancel">Cancel</button>
        <button type="button" class="fe-btn fe-btn-primary" id="fe-modal-ok">OK</button>
      </div>
    </div>
  </div>

  <footer class="portal-footer">
    <span>AcuGIS Portal &middot; Unified Spatial Workspace</span>
    <span class="footer-spacer"></span>
    <span class="footer-version">v0.1</span>
  </footer>

  <script src="https://cdnjs.cloudflare.com/ajax/libs/materialize/1.0.0/js/materialize.min.js"></script>
  <script>
  (function () {
    var root = document.getElementById('fe-root');
    if (!root) return;
    var API = root.getAttribute('data-api-base') || '';
    var canEdit = root.getAttribute('data-can-edit') === '1';
    var publishURL = root.getAttribute('data-publish-url') || '';
    var explorerURL = root.getAttribute('data-explorer-url') || (API + '/explorer/');
    var allowRecursive = root.getAttribute('data-allow-recursive') === '1';
    var currentFolderId = 0;
    var expanded = {};
    var favorites = {};
    var ctxEl = document.getElementById('fe-ctx');
    var modalBackdrop = document.getElementById('fe-modal-backdrop');
    var modalOk = document.getElementById('fe-modal-ok');
    var modalCancel = document.getElementById('fe-modal-cancel');
    var modalResolve = null;

    function api(path, opts) {
      opts = opts || {};
      opts.credentials = 'same-origin';
      opts.headers = Object.assign({'Accept': 'application/json'}, opts.headers || {});
      if (opts.body && typeof opts.body === 'object' && !(opts.body instanceof FormData)) {
        opts.headers['Content-Type'] = 'application/json';
        opts.body = JSON.stringify(opts.body);
      }
      return fetch(API + path, opts).then(function (res) {
        return res.json().then(function (data) {
          if (!res.ok) throw new Error((data && data.error) || res.statusText);
          return data;
        });
      });
    }

    function iconFor(folder) {
      return (folder && folder.icon) || 'folder';
    }

    function loadFavorites() {
      return api('/api/v1/favorites').then(function (data) {
        favorites = {};
        (data.items || []).forEach(function (it) {
          favorites[it.content_type + ':' + it.content_key] = true;
        });
      }).catch(function () {});
    }

    function renderTree(node, depth) {
      if (!node) return '';
      depth = depth || 0;
      var hasKids = node.children && node.children.length;
      var open = expanded[node.id] !== false; // default expanded
      if (expanded[node.id] === undefined && depth > 0) open = false;
      var html = '<li data-id="' + node.id + '">';
      html += '<div class="fe-tree-node' + (node.id === currentFolderId ? ' is-active' : '') + '" data-folder-id="' + node.id + '">';
      if (hasKids) {
        html += '<button type="button" class="fe-tree-toggle" data-toggle="' + node.id + '" aria-label="Toggle"><i class="material-icons">' + (open ? 'expand_more' : 'chevron_right') + '</i></button>';
      } else {
        html += '<span style="width:22px;display:inline-block;"></span>';
      }
      html += '<span class="fe-tree-label"><i class="material-icons">' + esc(iconFor(node)) + '</i><span>' + esc(node.name) + '</span></span>';
      html += '</div>';
      if (hasKids) {
        html += '<ul class="fe-tree-children' + (open ? '' : ' is-collapsed') + '">';
        node.children.forEach(function (c) { html += renderTree(c, depth + 1); });
        html += '</ul>';
      }
      html += '</li>';
      return html;
    }

    function loadTree() {
      return api('/api/v1/folders/tree').then(function (tree) {
        if (tree && tree.id && !currentFolderId) currentFolderId = tree.id;
        expanded[tree.id] = true;
        document.getElementById('fe-tree-mount').innerHTML = '<ul>' + renderTree(tree) + '</ul>';
      });
    }

    function renderBreadcrumbs(crumbs) {
      var el = document.getElementById('fe-breadcrumbs');
      if (!crumbs || !crumbs.length) { el.innerHTML = ''; return; }
      el.innerHTML = crumbs.map(function (c, i) {
        if (i === crumbs.length - 1) return '<span>' + esc(c.name) + '</span>';
        return '<a href="#" data-folder-id="' + c.id + '">' + esc(c.name) + '</a><span class="sep">›</span>';
      }).join('');
    }

    function rowHTML(kind, data) {
      var isFolder = kind === 'folder';
      var id = isFolder ? data.id : data.content_key;
      var name = isFolder ? data.name : (data.display_name || data.content_key);
      var desc = isFolder ? (data.description || '') : '';
      var typeLabel = 'Item';
      var icon = 'insert_drive_file';
      var iconClass = '';
      if (isFolder) {
        typeLabel = 'Folder';
        icon = iconFor(data);
      } else if (data.content_type === 'jasper_report') {
        typeLabel = 'Report';
        icon = 'assessment';
        iconClass = ' is-report';
      } else if (data.content_type === 'explorer_view') {
        if (data.resource_type === 'dashboard') {
          typeLabel = 'Dashboard';
          icon = 'dashboard';
          iconClass = ' is-dashboard';
        } else {
          typeLabel = 'View';
          icon = 'travel_explore';
          iconClass = ' is-view';
        }
      } else if (data.content_type === 'content_resource') {
        typeLabel = resourceTypeLabel(data.resource_type);
        icon = resourceIcon(data.resource_type);
        iconClass = data.resource_type === 'image' ? ' is-image' : ' is-resource';
        var meta = [];
          if (data.resource_type === 'image') {
            var fmt = (data.extension || '').replace(/^\./, '').toUpperCase() || 'Image';
            meta.push(fmt);
            if (data.resource_path) meta.push(data.resource_path);
            if (data.width && data.height) meta.push(data.width + '×' + data.height);
          if (data.size_bytes) meta.push(formatSize(data.size_bytes));
          if (data.updated_at) meta.push(formatDate(data.updated_at));
        } else {
          if (data.original_filename) meta.push(data.original_filename);
          if (data.size_bytes) meta.push(formatSize(data.size_bytes));
          if (data.updated_at) meta.push(formatDate(data.updated_at));
        }
        desc = meta.join(' · ');
      } else {
        typeLabel = data.content_type || 'Item';
      }
      var favKey = isFolder ? ('content_folder:' + data.id) : (data.content_type + ':' + data.content_key);
      var star = favorites[favKey] ? 'star' : 'star_border';
      var iconHTML;
      if (!isFolder && data.content_type === 'content_resource' && data.resource_type === 'image') {
        iconHTML = '<div class="fe-row-icon is-image"><img src="' + API + '/api/v1/resources/' +
          encodeURIComponent(data.content_key) + '/thumbnail" alt="" loading="lazy" onerror="this.style.display=\'none\';this.nextElementSibling.style.display=\'inline\'">' +
          '<i class="material-icons" style="display:none">image</i></div>';
      } else {
        iconHTML = '<div class="fe-row-icon' + iconClass + '"><i class="material-icons">' + icon + '</i></div>';
      }
      return (
        '<div class="fe-row" draggable="' + (canEdit ? 'true' : 'false') + '"' +
        ' data-kind="' + kind + '"' +
        ' data-id="' + escAttr(String(id)) + '"' +
        (isFolder ? '' : ' data-content-type="' + escAttr(data.content_type) + '" data-content-key="' + escAttr(data.content_key) + '"') +
        (data.resource_type ? ' data-resource-type="' + escAttr(data.resource_type) + '"' : '') +
        (data.can_design ? ' data-can-design="1"' : '') +
        ' data-name="' + escAttr(name) + '">' +
          iconHTML +
          '<div><div class="fe-row-name">' + esc(name) + (favorites[favKey] ? ' ★' : '') + '</div>' +
          (desc ? '<div class="fe-row-desc">' + esc(desc) + '</div>' : '') + '</div>' +
          '<div class="fe-row-path"></div>' +
          '<div class="fe-row-type">' + esc(typeLabel) + '</div>' +
          '<button type="button" class="fe-row-menu" data-menu aria-label="Actions"><i class="material-icons">more_vert</i></button>' +
        '</div>'
      );
    }

    function resourceTypeLabel(t) {
      switch (t) {
        case 'public_key': return 'Public Key';
        case 'private_key': return 'Private Key';
        case 'client_certificate': return 'Client Certificate';
        case 'ca_certificate': return 'CA Certificate';
        case 'generic_file': return 'Generic File';
        case 'service_account_json': return 'Service Account JSON';
        case 'image': return 'Image';
        default: return t || 'Resource';
      }
    }
    function resourceIcon(t) {
      switch (t) {
        case 'public_key':
        case 'private_key':
        case 'service_account_json': return 'vpn_key';
        case 'client_certificate':
        case 'ca_certificate': return 'verified_user';
        case 'image': return 'image';
        default: return 'attachment';
      }
    }
    function formatSize(n) {
      n = Number(n) || 0;
      if (n < 1024) return n + ' B';
      if (n < 1048576) return (n / 1024).toFixed(1) + ' KB';
      return (n / 1048576).toFixed(1) + ' MB';
    }
    function formatDate(iso) {
      try {
        var d = new Date(iso);
        if (isNaN(d.getTime())) return '';
        return d.toLocaleDateString();
      } catch (_e) { return ''; }
    }

    function browse(folderId) {
      currentFolderId = folderId || 0;
      var q = folderId ? ('?folder_id=' + encodeURIComponent(folderId)) : '';
      return api('/api/v1/folders/browse' + q).then(function (data) {
        currentFolderId = data.folder.id;
        try {
          var url = new URL(location.href);
          if (currentFolderId) url.searchParams.set('folder_id', String(currentFolderId));
          else url.searchParams.delete('folder_id');
          history.replaceState(null, '', url.pathname + url.search + url.hash);
        } catch (_e) {}
        renderBreadcrumbs(data.breadcrumbs || []);
        var html = '';
        (data.folders || []).forEach(function (f) { html += rowHTML('folder', f); });
        (data.items || []).forEach(function (it) { html += rowHTML('item', it); });
        var list = document.getElementById('fe-list');
        if (!html) {
          list.innerHTML = '<div class="fe-empty"><i class="material-icons">folder_open</i><p>This folder is empty.</p></div>';
        } else {
          list.innerHTML = html;
        }
        loadTree();
      }).catch(function (err) {
        document.getElementById('fe-list').innerHTML = '<div class="fe-empty"><p>' + esc(err.message) + '</p></div>';
      });
    }

    function search(q) {
      if (!q) return browse(currentFolderId);
      return api('/api/v1/folders/search?q=' + encodeURIComponent(q)).then(function (data) {
        renderBreadcrumbs([{id: 0, name: 'Search results'}]);
        var html = '';
        (data.items || []).forEach(function (h) {
          var pathCell = '<div class="fe-row-path">' + esc(h.folder_path || '') + '</div>';
          if (h.kind === 'folder') {
            html += '<div class="fe-row" data-kind="folder" data-id="' + h.id + '" data-name="' + escAttr(h.name) + '">' +
              '<div class="fe-row-icon"><i class="material-icons">' + esc(h.icon || 'folder') + '</i></div>' +
              '<div><div class="fe-row-name">' + esc(h.name) + '</div></div>' +
              pathCell +
              '<div class="fe-row-type">Folder</div>' +
              '<button type="button" class="fe-row-menu" data-menu aria-label="Actions"><i class="material-icons">more_vert</i></button></div>';
          } else {
            var searchIcon = 'assessment';
            var searchIconClass = ' is-report';
            var searchType = h.content_type || 'Item';
            var searchRt = h.resource_type || '';
            if (h.content_type === 'content_resource') {
              searchIcon = 'vpn_key';
              searchIconClass = ' is-resource';
              searchType = 'Resource';
            } else if (h.content_type === 'jasper_report') {
              searchType = 'Report';
            } else if (h.content_type === 'explorer_view') {
              if (searchRt === 'dashboard') {
                searchIcon = 'dashboard';
                searchIconClass = ' is-dashboard';
                searchType = 'Dashboard';
              } else {
                searchIcon = 'travel_explore';
                searchIconClass = ' is-view';
                searchType = 'View';
              }
            }
            html += '<div class="fe-row" data-kind="item" data-id="' + escAttr(h.content_key) + '" data-content-type="' + escAttr(h.content_type) + '" data-content-key="' + escAttr(h.content_key) + '"' +
              (searchRt ? ' data-resource-type="' + escAttr(searchRt) + '"' : '') +
              (h.can_design ? ' data-can-design="1"' : '') +
              ' data-name="' + escAttr(h.name) + '">' +
              '<div class="fe-row-icon' + searchIconClass + '"><i class="material-icons">' + searchIcon + '</i></div>' +
              '<div><div class="fe-row-name">' + esc(h.name) + '</div></div>' +
              pathCell +
              '<div class="fe-row-type">' + esc(searchType) + '</div>' +
              '<button type="button" class="fe-row-menu" data-menu aria-label="Actions"><i class="material-icons">more_vert</i></button></div>';
          }
        });
        var list = document.getElementById('fe-list');
        list.innerHTML = html || '<div class="fe-empty"><i class="material-icons">search_off</i><p>No matching content.</p></div>';
      });
    }

    function openExplorerViewPermissions(ref, displayName) {
      var parts = String(ref || '').split('/');
      if (parts.length < 2) {
        alert('Invalid view reference.');
        return;
      }
      var owner = parts[0];
      var viewKey = parts.slice(1).join('/');
      var path = '/api/v1/explorer/view-permissions?owner=' + encodeURIComponent(owner) +
        '&key=' + encodeURIComponent(viewKey);
      api(path).then(function (data) {
        if (data.can_edit === false) {
          alert('You do not have permission to manage Access Groups for this view.');
          return;
        }
        var selected = {};
        (data.group_names || []).forEach(function (g) { selected[String(g).toLowerCase()] = true; });
        var available = data.available_groups || [];
        if (!available.length) {
          available = data.group_names || [];
        }
        var checks = available.map(function (g) {
          if (String(g).toLowerCase() === 'administrators') return '';
          var sel = selected[String(g).toLowerCase()] ? ' checked' : '';
          return '<label style="display:flex;align-items:center;gap:10px;padding:8px 4px;cursor:pointer;font-size:14px;">' +
            '<input type="checkbox" class="fe-acl-group" value="' + escAttr(g) + '"' + sel + '>' +
            '<span>' + esc(g) + '</span></label>';
        }).filter(Boolean).join('');
        var body = '<p class="muted-small" style="margin:0 0 10px;">' + esc(displayName || ref) + '</p>' +
          '<label style="display:block;font-size:12px;color:#78909c;margin:0 0 6px;">Access Groups</label>' +
          '<div id="fe-modal-acl-groups" style="border:1px solid #cfd8dc;border-radius:6px;padding:8px 10px;max-height:220px;overflow:auto;">' +
          (checks || '<p class="muted-small">No access groups are configured.</p>') +
          '</div>' +
          '<p class="muted-small" style="margin:8px 0 0;">Select one or more groups. Empty selection defaults to Public.</p>';
        return openModal('Permissions', body, {okLabel: 'Save'}).then(function (val) {
          if (val == null) return;
          var names = Array.isArray(val) ? val : [];
          return api(path, {method: 'PUT', body: {group_names: names}});
        });
      }).catch(function (err) { alert(err.message); });
    }

    function openModal(title, bodyHTML, opts) {
      opts = opts || {};
      document.getElementById('fe-modal-title').textContent = title;
      document.getElementById('fe-modal-body').innerHTML = bodyHTML;
      var modal = document.getElementById('fe-modal');
      if (opts.wide) modal.classList.add('fe-modal-wide');
      else modal.classList.remove('fe-modal-wide');
      modalOk.textContent = opts.okLabel || 'OK';
      modalBackdrop.style.display = 'flex';
      return new Promise(function (resolve) { modalResolve = resolve; });
    }
    function closeModal(val) {
      modalBackdrop.style.display = 'none';
      if (modalResolve) { modalResolve(val); modalResolve = null; }
    }
    modalCancel.addEventListener('click', function () { closeModal(null); });
    modalOk.addEventListener('click', function () {
      if (modalOk.getAttribute('data-wizard') === '1') {
        closeModal(collectWizard());
        return;
      }
      var multi = document.getElementById('fe-modal-multiselect');
      if (multi) {
        var picked = [];
        Array.prototype.forEach.call(multi.options, function (o) {
          if (o.selected) picked.push(o.value);
        });
        closeModal(picked);
        return;
      }
      var aclBox = document.getElementById('fe-modal-acl-groups');
      if (aclBox) {
        var pickedAcl = [];
        Array.prototype.forEach.call(aclBox.querySelectorAll('input.fe-acl-group'), function (cb) {
          if (cb.checked) pickedAcl.push(cb.value);
        });
        closeModal(pickedAcl);
        return;
      }
      var input = document.getElementById('fe-modal-input');
      var select = document.getElementById('fe-modal-select');
      if (select) closeModal(select.value);
      else if (input) closeModal(input.value);
      else closeModal(true);
    });

    function collectWizard() {
      var nameEl = document.getElementById('fe-res-name');
      var typeEl = document.querySelector('input[name="fe-res-type"]:checked');
      var mode = document.getElementById('fe-res-mode');
      var paste = document.getElementById('fe-res-paste');
      var file = document.getElementById('fe-res-file');
      return {
        display_name: nameEl ? nameEl.value.trim() : '',
        resource_type: typeEl ? typeEl.value : '',
        mode: mode ? mode.value : 'upload',
        content: paste ? paste.value : '',
        file: file && file.files && file.files[0] ? file.files[0] : null
      };
    }

    function hideCtx() { ctxEl.style.display = 'none'; }
    function showCtx(x, y, items) {
      ctxEl.innerHTML = items.map(function (it) {
        return '<button type="button" class="' + (it.danger ? 'danger' : '') + '" data-action="' + it.action + '">' +
          '<i class="material-icons">' + it.icon + '</i>' + esc(it.label) + '</button>';
      }).join('');
      ctxEl.style.display = 'block';
      ctxEl.style.left = Math.min(x, window.innerWidth - 200) + 'px';
      ctxEl.style.top = Math.min(y, window.innerHeight - 240) + 'px';
    }

    var ctxTarget = null;
    document.addEventListener('click', hideCtx);
    ctxEl.addEventListener('click', function (e) {
      e.stopPropagation();
      var btn = e.target.closest('[data-action]');
      if (!btn || !ctxTarget) return;
      var action = btn.getAttribute('data-action');
      hideCtx();
      handleAction(action, ctxTarget);
    });

    function handleAction(action, row) {
      var kind = row.getAttribute('data-kind');
      var id = row.getAttribute('data-id');
      var name = row.getAttribute('data-name') || '';
      var ct = row.getAttribute('data-content-type') || '';
      var ck = row.getAttribute('data-content-key') || '';

      if (action === 'open') {
        if (kind === 'folder') browse(parseInt(id, 10));
        else if (ct === 'jasper_report') {
          try { sessionStorage.setItem('jrsCatalogReturn', location.href); } catch (_e) {}
          window.location.href = API + '/reports/' + encodeURIComponent(ck);
        } else if (ct === 'explorer_view') {
          var mode = (row.getAttribute('data-resource-type') === 'dashboard') ? 'dashboard' : 'edit';
          window.location.href = API + '/explorer/?mode=' + mode + '&view=' + encodeURIComponent(ck);
        } else if (ct === 'content_resource') {
          if (row.getAttribute('data-resource-type') === 'image') openImagePreview(ck);
          else openResourceDetail(ck);
        }
        return;
      }
      if (action === 'design') {
        if (ct === 'jasper_report' && ck) {
          window.location.href = API + '/reports/' + encodeURIComponent(ck) + '/design';
        }
        return;
      }
      if (action === 'edit') {
        if (kind === 'folder') {
          handleAction('rename', row);
          return;
        }
        if (ct === 'jasper_report') {
          window.location.href = API + '/reports/edit_report?report=' + encodeURIComponent(ck);
        } else if (ct === 'explorer_view') {
          window.location.href = API + '/explorer/?mode=edit&view=' + encodeURIComponent(ck);
        } else if (ct === 'content_resource') {
          if (row.getAttribute('data-resource-type') === 'image') openImagePreview(ck);
          else openResourceDetail(ck);
        }
        return;
      }
      if (action === 'download') {
        if (ct === 'content_resource') {
          window.location.href = API + '/api/v1/resources/' + encodeURIComponent(ck) + '/download';
        }
        return;
      }
      if (action === 'rename') {
        openModal('Rename', '<input id="fe-modal-input" value="' + escAttr(name) + '">').then(function (val) {
          if (!val || !val.trim()) return;
          if (kind === 'folder') {
            return api('/api/v1/folders/' + id, {method: 'PUT', body: {name: val.trim()}}).then(function () { browse(currentFolderId); });
          }
          if (ct === 'content_resource') {
            return api('/api/v1/resources/' + encodeURIComponent(ck), {method: 'PUT', body: {display_name: val.trim()}})
              .then(function () { browse(currentFolderId); })
              .catch(function (err) { alert(err.message); });
          }
          alert('Rename reports from Edit report.');
        });
        return;
      }
      if (action === 'move') {
        openModal('Move to folder ID', '<input id="fe-modal-input" type="number" placeholder="Destination folder id" value="' + currentFolderId + '">').then(function (val) {
          var dest = parseInt(val, 10);
          if (!dest) return;
          if (kind === 'folder') {
            return api('/api/v1/folders/' + id + '/move', {method: 'POST', body: {parent_id: dest}}).then(function () { browse(currentFolderId); loadTree(); });
          }
          return api('/api/v1/content/move', {method: 'POST', body: {folder_id: dest, content_type: ct, content_key: ck, display_name: name}}).then(function () { browse(currentFolderId); });
        });
        return;
      }
      if (action === 'delete') {
        if (kind === 'folder') {
          var msg = allowRecursive
            ? 'Delete folder "' + name + '" and all contents? This cannot be undone.'
            : 'Delete folder "' + name + '"? It must be empty.';
          if (!confirm(msg)) return;
          var qs = allowRecursive ? '?recursive=1&confirm=1' : '';
          api('/api/v1/folders/' + id + qs, {method: 'DELETE'}).then(function () { browse(currentFolderId); loadTree(); }).catch(function (err) { alert(err.message); });
        } else if (ct === 'jasper_report') {
          if (!confirm('Delete report "' + name + '"?')) return;
          window.location.href = API + '/reports/delete_report?report=' + encodeURIComponent(ck);
        } else if (ct === 'explorer_view') {
          if (!confirm('Delete saved view "' + name + '"? This cannot be undone.')) return;
          var parts = String(ck).split('/');
          var viewKey = parts.length > 1 ? parts.slice(1).join('/') : ck;
          var ownerQs = parts.length > 1 ? ('?owner=' + encodeURIComponent(parts[0])) : '';
          api('/api/v1/explorer/views/' + encodeURIComponent(viewKey) + ownerQs, {method: 'DELETE'})
            .then(function () { browse(currentFolderId); })
            .catch(function (err) { alert(err.message); });
        } else if (ct === 'content_resource') {
          if (!confirm('Delete resource "' + name + '"?')) return;
          api('/api/v1/resources/' + encodeURIComponent(ck), {method: 'DELETE'})
            .then(function () { browse(currentFolderId); })
            .catch(function (err) { alert(err.message); });
        }
        return;
      }
      if (action === 'permissions') {
        if (kind !== 'folder') {
          if (ct === 'content_resource') {
            alert('Resources inherit permissions from their folder.');
            return;
          }
          if (ct === 'explorer_view') {
            openExplorerViewPermissions(ck, name);
            return;
          }
          window.location.href = API + '/reports/edit_report?report=' + encodeURIComponent(ck);
          return;
        }
        api('/api/v1/folders/' + id + '/permissions').then(function (data) {
          var groups = (data.group_names || []).join(', ');
          return openModal('Folder permissions (comma-separated groups)', '<input id="fe-modal-input" value="' + escAttr(groups) + '">').then(function (val) {
            if (val == null) return;
            var names = String(val).split(',').map(function (s) { return s.trim(); }).filter(Boolean);
            return api('/api/v1/folders/' + id + '/permissions', {method: 'PUT', body: {group_names: names, permission: 'view'}});
          });
        }).catch(function (err) { alert(err.message); });
        return;
      }
      if (action === 'duplicate') {
        if (kind === 'folder') {
          api('/api/v1/folders/' + id + '/duplicate', {method: 'POST', body: {}}).then(function () { browse(currentFolderId); loadTree(); }).catch(function (err) { alert(err.message); });
        } else {
          alert('Duplicate report from Publish / Edit for now.');
        }
        return;
      }
      if (action === 'favorite') {
        var favKey = kind === 'folder' ? ('content_folder:' + id) : (ct + ':' + ck);
        var on = !favorites[favKey];
        api('/api/v1/favorites', {method: 'POST', body: {
          content_type: kind === 'folder' ? 'content_folder' : ct,
          content_key: kind === 'folder' ? String(id) : ck,
          favorite: on
        }}).then(function () { return loadFavorites(); }).then(function () { browse(currentFolderId); });
      }
    }

    document.getElementById('fe-list').addEventListener('dblclick', function (e) {
      var row = e.target.closest('.fe-row');
      if (!row) return;
      handleAction('open', row);
    });
    document.getElementById('fe-list').addEventListener('click', function (e) {
      var menu = e.target.closest('[data-menu]');
      if (!menu) return;
      e.preventDefault();
      e.stopPropagation();
      var row = menu.closest('.fe-row');
      ctxTarget = row;
      var kind = row.getAttribute('data-kind');
      var items = [
        {action: 'open', icon: 'open_in_browser', label: 'Open'}
      ];
      if (kind !== 'folder') {
        items.push({action: 'edit', icon: 'edit', label: 'Edit'});
      }
      if (kind !== 'folder' && row.getAttribute('data-content-type') === 'jasper_report' && row.getAttribute('data-can-design') === '1') {
        items.push({action: 'design', icon: 'design_services', label: 'Edit in Designer'});
      }
      if (kind !== 'folder' && row.getAttribute('data-content-type') === 'content_resource') {
        items.push({action: 'download', icon: 'download', label: 'Download'});
      }
      items = items.concat([
        {action: 'rename', icon: 'drive_file_rename_outline', label: 'Rename'},
        {action: 'move', icon: 'drive_file_move', label: 'Move'},
        {action: 'permissions', icon: 'lock', label: 'Permissions'},
        {action: 'duplicate', icon: 'content_copy', label: 'Duplicate'},
        {action: 'favorite', icon: 'star', label: 'Favorite'},
        {action: 'delete', icon: 'delete', label: 'Delete', danger: true}
      ]);
      if (!canEdit) items = items.filter(function (i) { return i.action === 'open' || i.action === 'favorite' || i.action === 'download'; });
      showCtx(e.clientX, e.clientY, items);
    });

    document.getElementById('fe-tree-mount').addEventListener('click', function (e) {
      var toggle = e.target.closest('[data-toggle]');
      if (toggle) {
        var tid = toggle.getAttribute('data-toggle');
        expanded[tid] = !(expanded[tid] !== false);
        loadTree();
        return;
      }
      var node = e.target.closest('[data-folder-id]');
      if (node) browse(parseInt(node.getAttribute('data-folder-id'), 10));
    });

    document.getElementById('fe-breadcrumbs').addEventListener('click', function (e) {
      var a = e.target.closest('[data-folder-id]');
      if (!a) return;
      e.preventDefault();
      browse(parseInt(a.getAttribute('data-folder-id'), 10));
    });

    var searchTimer = null;
    document.getElementById('fe-search').addEventListener('input', function () {
      var q = this.value.trim();
      clearTimeout(searchTimer);
      searchTimer = setTimeout(function () { search(q); }, 250);
    });

    var newFolderBtn = document.getElementById('fe-new-folder');
    if (newFolderBtn) {
      newFolderBtn.addEventListener('click', function () {
        openModal('New Folder', '<input id="fe-modal-input" placeholder="Folder name" autofocus>').then(function (val) {
          if (!val || !val.trim()) return;
          return api('/api/v1/folders', {method: 'POST', body: {parent_id: currentFolderId, name: val.trim()}})
            .then(function () { browse(currentFolderId); loadTree(); })
            .catch(function (err) { alert(err.message); });
        });
      });
    }

    function createFolder() {
      openModal('New Folder', '<input id="fe-modal-input" placeholder="Folder name" autofocus>').then(function (val) {
        if (!val || !val.trim()) return;
        return api('/api/v1/folders', {method: 'POST', body: {parent_id: currentFolderId, name: val.trim()}})
          .then(function () { browse(currentFolderId); loadTree(); })
          .catch(function (err) { alert(err.message); });
      });
    }

    function openResourceWizard() {
      var types = [
        {v: 'public_key', l: 'Public Key'},
        {v: 'private_key', l: 'Private Key'},
        {v: 'client_certificate', l: 'Client Certificate'},
        {v: 'ca_certificate', l: 'CA Certificate'},
        {v: 'generic_file', l: 'Generic File'},
        {v: 'service_account_json', l: 'Service Account JSON'},
        {v: 'image', l: 'Image'}
      ];
      var radios = types.map(function (t, i) {
        return '<label><input type="radio" name="fe-res-type" value="' + t.v + '"' + (i === 0 ? ' checked' : '') + '> ' + t.l + '</label>';
      }).join('');
      var html =
        '<div class="fe-wizard-step"><label for="fe-res-name">Display Name</label>' +
        '<input id="fe-res-name" placeholder="Snowflake Production Key" autofocus></div>' +
        '<div class="fe-wizard-step"><label>Resource Type</label><div class="fe-wizard-radios">' + radios + '</div></div>' +
        '<div class="fe-wizard-step"><label>Content</label>' +
        '<div class="fe-wizard-tabs" id="fe-res-tabs">' +
        '<button type="button" class="fe-btn is-active" data-res-mode="upload">Upload File</button>' +
        '<button type="button" class="fe-btn" data-res-mode="paste" id="fe-res-paste-tab">Paste Text</button>' +
        '</div>' +
        '<input type="hidden" id="fe-res-mode" value="upload">' +
        '<div id="fe-res-upload-pane">' +
        '<div class="fe-dropzone" id="fe-res-dropzone" tabindex="0">' +
        '<i class="material-icons">cloud_upload</i>' +
        '<div>Drag &amp; drop a file here, or click to browse</div>' +
        '<div class="fe-dropzone-file" id="fe-res-drop-label"></div>' +
        '</div>' +
        '<input type="file" id="fe-res-file" style="display:none" accept=".png,.jpg,.jpeg,.gif,.svg,.webp,.pem,.pub,.crt,.key,.txt,.bin,image/*">' +
        '</div>' +
        '<div id="fe-res-paste-pane" style="display:none"><textarea id="fe-res-paste" placeholder="-----BEGIN …-----"></textarea></div>' +
        '</div>';
      modalOk.setAttribute('data-wizard', '1');
      openModal('New Resource', html, {wide: true, okLabel: 'Create'}).then(function (payload) {
        modalOk.removeAttribute('data-wizard');
        if (!payload) return;
        if (!payload.display_name) { alert('Display name is required.'); return; }
        if (!payload.resource_type) { alert('Select a resource type.'); return; }
        var fd = new FormData();
        fd.append('display_name', payload.display_name);
        fd.append('resource_type', payload.resource_type);
        fd.append('folder_id', String(currentFolderId || 0));
        var endpoint = API + '/api/v1/resources';
        if (payload.resource_type === 'image') {
          endpoint = API + '/api/v1/resources/images';
          if (!payload.file) { alert('Choose an image file to upload.'); return; }
          fd.append('file', payload.file, payload.file.name);
        } else if (payload.mode === 'paste') {
          if (!payload.content || !String(payload.content).trim()) { alert('Paste PEM or text content.'); return; }
          fd.append('content', payload.content);
        } else {
          if (!payload.file) { alert('Choose a file to upload.'); return; }
          fd.append('file', payload.file, payload.file.name);
        }
        return fetch(endpoint, {method: 'POST', credentials: 'same-origin', body: fd})
          .then(function (res) {
            return res.json().then(function (data) {
              if (!res.ok) throw new Error((data && data.error) || res.statusText);
              return data;
            });
          })
          .then(function () { browse(currentFolderId); })
          .catch(function (err) { alert(err.message); });
      }).catch(function () { modalOk.removeAttribute('data-wizard'); });
      setTimeout(function () {
        var fileInput = document.getElementById('fe-res-file');
        var dropzone = document.getElementById('fe-res-dropzone');
        var dropLabel = document.getElementById('fe-res-drop-label');
        var pasteTab = document.getElementById('fe-res-paste-tab');
        var tabs = document.querySelectorAll('[data-res-mode]');
        function selectedType() {
          var el = document.querySelector('input[name="fe-res-type"]:checked');
          return el ? el.value : '';
        }
        function syncTypeUI() {
          var isImage = selectedType() === 'image';
          if (pasteTab) pasteTab.style.display = isImage ? 'none' : '';
          if (isImage) {
            document.getElementById('fe-res-mode').value = 'upload';
            document.getElementById('fe-res-upload-pane').style.display = '';
            document.getElementById('fe-res-paste-pane').style.display = 'none';
            tabs.forEach(function (b) {
              b.classList.toggle('is-active', b.getAttribute('data-res-mode') === 'upload');
            });
            if (fileInput) fileInput.accept = '.png,.jpg,.jpeg,.gif,.svg,.webp,image/png,image/jpeg,image/gif,image/svg+xml,image/webp';
          } else if (fileInput) {
            fileInput.accept = '';
          }
        }
        document.querySelectorAll('input[name="fe-res-type"]').forEach(function (r) {
          r.addEventListener('change', syncTypeUI);
        });
        syncTypeUI();
        tabs.forEach(function (btn) {
          btn.addEventListener('click', function () {
            if (selectedType() === 'image' && btn.getAttribute('data-res-mode') === 'paste') return;
            tabs.forEach(function (b) { b.classList.remove('is-active'); });
            btn.classList.add('is-active');
            var mode = btn.getAttribute('data-res-mode');
            document.getElementById('fe-res-mode').value = mode;
            document.getElementById('fe-res-upload-pane').style.display = mode === 'upload' ? '' : 'none';
            document.getElementById('fe-res-paste-pane').style.display = mode === 'paste' ? '' : 'none';
          });
        });
        function setFile(f) {
          if (!f || !fileInput) return;
          try {
            var dt = new DataTransfer();
            dt.items.add(f);
            fileInput.files = dt.files;
          } catch (_e) {}
          if (dropLabel) dropLabel.textContent = f.name + ' (' + formatSize(f.size) + ')';
          var nameEl = document.getElementById('fe-res-name');
          if (nameEl && !nameEl.value.trim()) {
            nameEl.value = f.name.replace(/\.[^.]+$/, '');
          }
        }
        if (dropzone && fileInput) {
          dropzone.addEventListener('click', function () { fileInput.click(); });
          fileInput.addEventListener('change', function () {
            if (fileInput.files && fileInput.files[0]) setFile(fileInput.files[0]);
          });
          ['dragenter', 'dragover'].forEach(function (ev) {
            dropzone.addEventListener(ev, function (e) {
              e.preventDefault(); e.stopPropagation();
              dropzone.classList.add('is-dragover');
            });
          });
          ['dragleave', 'drop'].forEach(function (ev) {
            dropzone.addEventListener(ev, function (e) {
              e.preventDefault(); e.stopPropagation();
              dropzone.classList.remove('is-dragover');
            });
          });
          dropzone.addEventListener('drop', function (e) {
            var f = e.dataTransfer && e.dataTransfer.files && e.dataTransfer.files[0];
            if (f) setFile(f);
          });
        }
      }, 0);
    }

    function openResourceDetail(uuid) {
      api('/api/v1/resources/' + encodeURIComponent(uuid)).then(function (res) {
        if (res.resource_type === 'image') {
          openImagePreview(uuid, res);
          return;
        }
        var html =
          '<p><strong>' + esc(res.display_name) + '</strong></p>' +
          '<p class="fe-row-desc">' + esc(resourceTypeLabel(res.resource_type)) +
          (res.original_filename ? ' · ' + esc(res.original_filename) : '') +
          (res.size_bytes ? ' · ' + esc(formatSize(res.size_bytes)) : '') + '</p>' +
          (res.folder_path ? '<p class="fe-row-desc">' + esc(res.folder_path) + '</p>' : '') +
          '<p class="fe-row-desc">UUID: <code>' + esc(res.uuid) + '</code></p>';
        return openModal('Resource', html, {okLabel: 'Download'}).then(function (ok) {
          if (ok) window.location.href = API + '/api/v1/resources/' + encodeURIComponent(uuid) + '/download';
        });
      }).catch(function (err) { alert(err.message); });
    }

    function openImagePreview(uuid, preloaded) {
      var load = preloaded ? Promise.resolve(preloaded) : api('/api/v1/resources/images/' + encodeURIComponent(uuid));
      load.then(function (res) {
        var dims = (res.width && res.height) ? (res.width + ' × ' + res.height) : '—';
        var bust = res.updated_at ? encodeURIComponent(res.updated_at) : String(Date.now());
        var html =
          '<img class="fe-img-preview" src="' + API + '/api/v1/resources/' + encodeURIComponent(uuid) + '/download?inline=1&t=' + bust + '" alt="' + escAttr(res.display_name) + '">' +
          '<dl class="fe-img-meta">' +
          '<dt>Display Name</dt><dd>' + esc(res.display_name) + '</dd>' +
          '<dt>Original Filename</dt><dd>' + esc(res.original_filename || '—') + '</dd>' +
          '<dt>Dimensions</dt><dd>' + esc(dims) + '</dd>' +
          '<dt>File Size</dt><dd>' + esc(formatSize(res.size_bytes)) + '</dd>' +
          '<dt>MIME Type</dt><dd>' + esc(res.mime_type || '—') + '</dd>' +
          '<dt>Created</dt><dd>' + esc(formatDateTime(res.created_at)) + '</dd>' +
          '<dt>Modified</dt><dd>' + esc(formatDateTime(res.updated_at)) + '</dd>' +
          '</dl>' +
          '<div class="fe-img-actions">' +
          (canEdit ? '<button type="button" class="fe-btn" id="fe-img-replace"><i class="material-icons">swap_horiz</i> Replace</button>' : '') +
          '<button type="button" class="fe-btn" id="fe-img-download"><i class="material-icons">download</i> Download</button>' +
          (canEdit ? '<button type="button" class="fe-btn danger" id="fe-img-delete" style="color:#c62828"><i class="material-icons">delete</i> Delete</button>' : '') +
          '<input type="file" id="fe-img-replace-file" accept=".png,.jpg,.jpeg,.gif,.svg,.webp,image/*" style="display:none">' +
          '</div>';
        return openModal('Image', html, {wide: true, okLabel: 'Close'}).then(function () {});
      }).then(function () {
        /* modal closed */
      }).catch(function (err) { alert(err.message); });
      setTimeout(function () {
        var dl = document.getElementById('fe-img-download');
        if (dl) dl.addEventListener('click', function () {
          window.location.href = API + '/api/v1/resources/images/' + encodeURIComponent(uuid) + '/download';
        });
        var del = document.getElementById('fe-img-delete');
        if (del) del.addEventListener('click', function () {
          if (!confirm('Delete this image resource?')) return;
          api('/api/v1/resources/images/' + encodeURIComponent(uuid), {method: 'DELETE'})
            .then(function () { closeModal(null); browse(currentFolderId); })
            .catch(function (err) { alert(err.message); });
        });
        var repBtn = document.getElementById('fe-img-replace');
        var repFile = document.getElementById('fe-img-replace-file');
        if (repBtn && repFile) {
          repBtn.addEventListener('click', function () { repFile.click(); });
          repFile.addEventListener('change', function () {
            if (!repFile.files || !repFile.files[0]) return;
            var fd = new FormData();
            fd.append('file', repFile.files[0], repFile.files[0].name);
            fetch(API + '/api/v1/resources/images/' + encodeURIComponent(uuid), {
              method: 'PUT', credentials: 'same-origin', body: fd
            }).then(function (res) {
              return res.json().then(function (data) {
                if (!res.ok) throw new Error((data && data.error) || res.statusText);
                return data;
              });
            }).then(function () {
              closeModal(null);
              browse(currentFolderId);
              openImagePreview(uuid);
            }).catch(function (err) { alert(err.message); });
          });
        }
      }, 0);
    }

    function formatDateTime(iso) {
      try {
        var d = new Date(iso);
        if (isNaN(d.getTime())) return '—';
        return d.toLocaleString();
      } catch (_e) { return '—'; }
    }

    var newMenuBtn = document.getElementById('fe-new-menu-btn');
    var newMenu = document.getElementById('fe-new-menu');
    if (newMenuBtn && newMenu) {
      newMenuBtn.addEventListener('click', function (e) {
        e.stopPropagation();
        newMenu.classList.toggle('is-open');
      });
      document.addEventListener('click', function () { newMenu.classList.remove('is-open'); });
      newMenu.addEventListener('click', function (e) {
        var btn = e.target.closest('[data-new]');
        if (!btn) return;
        e.preventDefault();
        newMenu.classList.remove('is-open');
        var kind = btn.getAttribute('data-new');
        if (kind === 'folder') createFolder();
        else if (kind === 'resource') openResourceWizard();
        else if (kind === 'view' || kind === 'dashboard') {
          var q = 'intent=' + encodeURIComponent(kind);
          if (currentFolderId) q += '&folder_id=' + encodeURIComponent(String(currentFolderId));
          window.location.href = explorerURL + (explorerURL.indexOf('?') >= 0 ? '&' : '?') + q;
        }
      });
    }

    // Drag and drop
    var dragRow = null;
    document.getElementById('fe-list').addEventListener('dragstart', function (e) {
      var row = e.target.closest('.fe-row');
      if (!row || !canEdit) return;
      dragRow = row;
      row.classList.add('is-dragging');
      e.dataTransfer.setData('text/plain', row.getAttribute('data-id'));
      e.dataTransfer.effectAllowed = 'move';
    });
    document.getElementById('fe-list').addEventListener('dragend', function () {
      if (dragRow) dragRow.classList.remove('is-dragging');
      dragRow = null;
      document.querySelectorAll('.is-drop-target').forEach(function (el) { el.classList.remove('is-drop-target'); });
    });
    document.getElementById('fe-list').addEventListener('dragover', function (e) {
      var row = e.target.closest('.fe-row[data-kind="folder"]');
      if (!row || !dragRow || row === dragRow) return;
      e.preventDefault();
      row.classList.add('is-drop-target');
    });
    document.getElementById('fe-list').addEventListener('dragleave', function (e) {
      var row = e.target.closest('.fe-row');
      if (row) row.classList.remove('is-drop-target');
    });
    document.getElementById('fe-list').addEventListener('drop', function (e) {
      var target = e.target.closest('.fe-row[data-kind="folder"]');
      if (!target || !dragRow || !canEdit) return;
      e.preventDefault();
      var dest = parseInt(target.getAttribute('data-id'), 10);
      var kind = dragRow.getAttribute('data-kind');
      var p;
      if (kind === 'folder') {
        p = api('/api/v1/folders/' + dragRow.getAttribute('data-id') + '/move', {method: 'POST', body: {parent_id: dest}});
      } else {
        p = api('/api/v1/content/move', {method: 'POST', body: {
          folder_id: dest,
          content_type: dragRow.getAttribute('data-content-type'),
          content_key: dragRow.getAttribute('data-content-key'),
          display_name: dragRow.getAttribute('data-name')
        }});
      }
      p.then(function () { browse(currentFolderId); loadTree(); }).catch(function (err) { alert(err.message); });
    });

    // Allow dropping onto tree nodes
    document.getElementById('fe-tree-mount').addEventListener('dragover', function (e) {
      if (!dragRow) return;
      var node = e.target.closest('[data-folder-id]');
      if (!node) return;
      e.preventDefault();
    });
    document.getElementById('fe-tree-mount').addEventListener('drop', function (e) {
      var node = e.target.closest('[data-folder-id]');
      if (!node || !dragRow || !canEdit) return;
      e.preventDefault();
      var dest = parseInt(node.getAttribute('data-folder-id'), 10);
      var kind = dragRow.getAttribute('data-kind');
      var p;
      if (kind === 'folder') {
        p = api('/api/v1/folders/' + dragRow.getAttribute('data-id') + '/move', {method: 'POST', body: {parent_id: dest}});
      } else {
        p = api('/api/v1/content/move', {method: 'POST', body: {
          folder_id: dest,
          content_type: dragRow.getAttribute('data-content-type'),
          content_key: dragRow.getAttribute('data-content-key'),
          display_name: dragRow.getAttribute('data-name')
        }});
      }
      p.then(function () { browse(currentFolderId); loadTree(); }).catch(function (err) { alert(err.message); });
    });

    function esc(s) {
      return String(s == null ? '' : s).replace(/[&<>"']/g, function (c) {
        return ({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'})[c];
      });
    }
    function escAttr(s) { return esc(s); }

    loadFavorites().then(function () { return loadTree(); }).then(function () {
      var startFolder = 0;
      try {
        var fid = parseInt(new URLSearchParams(location.search).get('folder_id') || '0', 10);
        if (fid > 0) startFolder = fid;
      } catch (_e) {}
      return browse(startFolder);
    });
  })();
  </script>
</body>
</html>`

var catalogExplorerTemplate = template.Must(template.New("catalogExplorer").Parse(catalogExplorerHTML))
