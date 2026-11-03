/**
 * AcuGIS Report Server shell — single-product nav (stand-alone).
 * Override at runtime: window.FEDERATION_SHELL_CONFIG = { ... }
 */
(function (global) {
  'use strict';

  // Prefer runtime mount (Go sets FEDERATION_SHELL_MOUNT, including "" for root deploy).
  var phpMount = String(global.FEDERATION_PHP_MOUNT || '/acugis-report-server').replace(/\/$/, '') || '/acugis-report-server';
  var mount;
  if (Object.prototype.hasOwnProperty.call(global, 'FEDERATION_SHELL_MOUNT')) {
    // Explicit Go/PHP page bootstrap — empty string means site root (do NOT fall back to PHP mount).
    mount = String(global.FEDERATION_SHELL_MOUNT || '').replace(/\/$/, '');
  } else if (Object.prototype.hasOwnProperty.call(global, 'BASE_PATH')) {
    mount = String(global.BASE_PATH || '').replace(/\/$/, '');
  } else {
    mount = phpMount;
  }

  var reportItems = [
    {
      label: 'Reports',
      icon: 'assessment',
      href: 'reports/index.php',
      pathMatch: '(?:^|/)reports(?:/(?:index|edit_report))?(?:\\.php)?/?$',
      description: 'Browse and run published reports',
    },
    {
      label: 'Publish',
      icon: 'publish',
      href: 'reports/publish_report.php',
      pathMatch: '/reports/publish_report.php',
      description: 'Upload and publish new report templates',
    },
    {
      label: 'Create Designer Report',
      icon: 'design_services',
      href: 'reports/designer/new',
      pathMatch: '/reports/designer/new',
      requires: 'reportDesigner',
      description: 'Start a new report in Report Designer Web',
    },
    {
      label: 'Import Designer Project',
      icon: 'unarchive',
      href: 'reports/designer/import',
      pathMatch: '/reports/designer/import',
      requires: 'reportDesigner',
      description: 'Upload a desktop Designer project',
    },
  ];

  var explorerItems = [
    {
      label: 'Explore Data',
      icon: 'travel_explore',
      href: 'explorer/',
      pathMatch: '/explorer',
      feature: 'dataExplorer',
      description: 'Browse, filter, chart, and map datasets',
    },
    {
      label: 'My Views',
      icon: 'bookmark',
      href: 'explorer/?panel=views',
      pathMatch: '/explorer.*panel=views',
      feature: 'dataExplorer',
      description: 'Saved Data Explorer views',
    },
    {
      label: 'Dashboards',
      icon: 'dashboard',
      href: 'dashboards/',
      pathMatch: '/dashboards',
      feature: 'dashboards',
      // Top-level appNav entry owns active state; keep card on Ad Hoc hub only.
      activateHub: false,
      description: 'Data Explorer views designated as Dashboards',
    },
    {
      label: 'Data Quality',
      icon: 'insights',
      href: 'dataset-intelligence/',
      pathMatch: '/dataset-intelligence',
      feature: 'dataIntelligence',
      activateHub: false,
      description: 'Dataset structure, quality, spatial profile, and usage',
    },
  ];

  var schedulingItems = [
    {
      label: 'Schedules',
      icon: 'schedule',
      href: 'schedules.php',
      pathMatch: '/schedules.php',
      description: 'Automate report generation and delivery',
    },
    {
      label: 'Executions',
      icon: 'play_circle_outline',
      href: 'executions.php',
      pathMatch: '/executions.php',
      description: 'Recent run status per schedule',
    },
    {
      label: 'History',
      icon: 'history',
      href: 'history.php',
      pathMatch: '/history.php',
      description: 'Execution log and delivery outcomes',
    },
    {
      label: 'Recipients',
      icon: 'people',
      href: 'recipients.php',
      pathMatch: '/recipients.php',
      description: 'Email recipient directory',
    },
  ];

  var dataItems = [
    {
      label: 'Datasources',
      icon: 'storage',
      href: 'reports/datasources.php',
      pathMatch: '/reports/datasources.php',
      description: 'Configure report data sources',
    },
    {
      label: 'GeoServer',
      icon: 'public',
      href: 'reports/geoserver_connections.php',
      pathMatch: '/reports/geoserver_connections.php',
      description: 'GeoServer connections for Designer map publish',
    },
    {
      label: 'URL Builder',
      icon: 'link',
      href: 'reports/report_runner.php',
      pathMatch: '/reports/report_runner.php',
      description: 'Build parameterized report URLs',
    },
  ];

  var administrationItems = [
    {
      label: 'Users',
      icon: 'person',
      href: 'admin/users.php',
      pathMatch: '/admin/users.php',
      description: 'Manage local user accounts',
    },
    {
      label: 'Groups',
      icon: 'group',
      href: 'admin/groups.php',
      pathMatch: '/admin/groups.php',
      description: 'Manage groups and memberships',
    },
    {
      label: 'Authentication',
      icon: 'vpn_key',
      href: 'admin/authentication',
      pathMatch: '/admin/authentication',
      description: 'Local login and optional OIDC / SAML SSO',
    },
    {
      label: 'License',
      icon: 'verified',
      href: 'admin/license',
      pathMatch: '/admin/license',
      description: 'Activate AcuGIS Report Server Professional',
    },
    {
      label: 'Auditing',
      icon: 'fact_check',
      href: 'admin/audit.php',
      pathMatch: '/admin/audit',
      feature: 'audit',
      activateHub: false,
      description: 'Searchable audit trail and report performance',
    },
    {
      label: 'Usage Analytics',
      icon: 'analytics',
      href: 'admin/usage.php',
      pathMatch: '/admin/usage',
      feature: 'usageAnalytics',
      activateHub: false,
      description: 'Aggregated content, API, and embed usage',
    },
    {
      label: 'Governance',
      icon: 'verified',
      href: 'admin/governance.php',
      pathMatch: '/admin/governance',
      feature: 'governance',
      activateHub: false,
      description: 'Certification status, owners, and review dates',
    },
  ];

  var defaults = {
    assetsBase: '/shared/ui',
    portalHome: mount + '/hubs/reports.php',
    workspaceUrl: mount + '/hubs/reports.php',
    apps: {
      jasper: {
        id: 'jasper',
        label: 'AcuGIS Report Server',
        icon: 'assessment',
        description: 'Publish, schedule, and manage Jasper reports',
        mount: mount,
        match: [mount, '/jasper-report-services', '/jasper'],
        home: '/hubs/reports.php',
        workspace: '/hubs/reports.php',
        urlPrefix: function (p) {
          if (!p) return mount + '/';
          if (/^https?:\/\//i.test(p)) return p;
          if (p.charAt(0) === '/' && p.indexOf(mount) === 0) return p;
          var rel = p.charAt(0) === '/' ? p : '/' + p;
          return mount + rel;
        },
        sections: [
          { title: 'Reports', items: reportItems },
          { title: 'Ad Hoc', items: explorerItems },
          { title: 'Scheduling', items: schedulingItems },
          { title: 'Data Sources', items: dataItems },
          { title: 'Administration', items: administrationItems },
        ],
        hubs: {
          reports: {
            path: '/hubs/reports.php',
            label: 'Reports',
            parentLabel: 'AcuGIS Report Server',
            description: 'Browse, run, and publish report templates',
            // Prefix covers the catalog, publish, designer, and viewer.
            // Longer owns on the data hub win for datasource pages under /reports.
            owns: ['/reports'],
            sections: [{ title: 'Reports', items: reportItems }],
          },
          'data-explorer': {
            path: '/hubs/data-explorer.php',
            label: 'Ad Hoc',
            parentLabel: 'AcuGIS Report Server',
            description: 'Interactive browse, filter, chart, and map of datasets',
            owns: ['/explorer'],
            sections: [{ title: 'Ad Hoc', items: explorerItems }],
          },
          scheduling: {
            path: '/hubs/scheduling.php',
            label: 'Scheduling',
            parentLabel: 'AcuGIS Report Server',
            description: 'Automate report generation, delivery, and recipients',
            owns: ['/schedules', '/edit_schedule', '/executions', '/history', '/recipients', '/edit_recipient'],
            sections: [{ title: 'Scheduling', items: schedulingItems }],
          },
          data: {
            path: '/hubs/data.php',
            label: 'Data Sources',
            parentLabel: 'AcuGIS Report Server',
            description: 'Configure data sources and parameterized report URLs',
            owns: [
              '/reports/datasources',
              '/reports/edit_datasource',
              '/reports/geoserver_connections',
              '/reports/qgis_connections',
              '/reports/report_runner',
              '/reports/generate_jasper_context',
            ],
            sections: [{ title: 'Data Sources', items: dataItems }],
          },
          administration: {
            path: '/hubs/administration.php',
            label: 'Administration',
            parentLabel: 'AcuGIS Report Server',
            description: 'Manage users, groups, and audit trail',
            owns: ['/admin'],
            sections: [{ title: 'Administration', items: administrationItems }],
          },
        },
      },
    },
    globalNav: [],
    appNav: [
      { appId: 'jasper', hubId: 'reports', label: 'Reports', icon: 'assessment' },
      { appId: 'jasper', hubId: 'data-explorer', label: 'Ad Hoc', icon: 'travel_explore', tint: 'blue', feature: 'dataExplorer' },
      { appId: 'jasper', label: 'Dashboards', icon: 'dashboard', tint: 'teal', href: 'dashboards/', match: ['/dashboards'], feature: 'dashboards' },
      { appId: 'jasper', hubId: 'data', label: 'Data Sources', icon: 'storage', tint: 'teal' },
      { appId: 'jasper', hubId: 'scheduling', label: 'Scheduling', icon: 'schedule', tint: 'purple' },
      { appId: 'jasper', label: 'Data Quality', icon: 'insights', tint: 'blue', href: 'dataset-intelligence/', match: ['/dataset-intelligence'], feature: 'dataIntelligence' },
      { appId: 'jasper', label: 'Auditing', icon: 'fact_check', tint: 'grey', href: 'admin/audit.php', match: ['/admin/audit'], feature: 'audit' },
      {
        appId: 'jasper',
        hubId: 'administration',
        label: 'Administration',
        icon: 'admin_panel_settings',
        tint: 'grey',
      },
    ],
    footerNav: [
      {
        id: 'logout',
        label: 'Log Out',
        icon: 'logout',
        href: mount + '/logout.php',
        tint: 'grey',
      },
    ],
  };

  global.FEDERATION_SHELL_CONFIG = global.FEDERATION_SHELL_CONFIG || defaults;
})(typeof window !== 'undefined' ? window : this);
