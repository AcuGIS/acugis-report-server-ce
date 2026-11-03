<?php
declare(strict_types=1);

require_once __DIR__ . '/../lib/bootstrap.php';

// Local auth via app database (lib/auth.php).
$cconn = jrs_auth_require_central_db();
jrs_require_login($cconn);
jrs_require_service_access($cconn, 'jasper_reports', 'view');

if ($cconn) {
    jrs_sync_all_jasper_report_resources($cconn);
}

jrs_ensure_dir(jrs_json_dir());
jrs_ensure_dir(jrs_report_files_dir());
jrs_ensure_dir(jrs_report_assets_dir());

$reports = [];
foreach (jrs_filter_reports_for_user($cconn, jrs_all_reports(false)) as $json) {
    $reportKey = trim((string) ($json['report'] ?? ''));
    if ($reportKey === '') {
        continue;
    }

    $reportType = jrs_normalize_report_type($json['type'] ?? $json['reportType'] ?? '');
    $reports[] = [
        'name' => (string) ($json['name'] ?? 'Report'),
        'description' => trim((string) ($json['description'] ?? $json['summary'] ?? '')),
        'report' => $reportKey,
        'datasource' => (string) ($json['datasource'] ?? ''),
        'type' => $reportType,
        'type_label' => jrs_report_type_label($reportType),
        'thumbnail_exists' => is_file(jrs_report_asset_path($reportKey)),
    ];
}

$user = jrs_get_current_user();
$uid = (int) ($user['id'] ?? 0);
$username = (string) ($user['username'] ?? '');
$isAdminOrEditor = (bool) ($user['is_admin'] ?? false)
    || jrs_auth_groups_include_admin((array) ($user['groups'] ?? []))
    || jrs_has_permission($cconn, $uid, JRS_RESOURCE_TYPE_SERVICE, 'jasper_reports', 'edit');
$isPlatformAdmin = (bool) ($user['is_admin'] ?? false)
    || jrs_auth_groups_include_admin((array) ($user['groups'] ?? []));

$mount = rtrim(jrs_app_mount(), '/');
$adminUrl = $mount . '/hubs/reports.php';
$accountUrl = $mount . '/profile.php';
$logoutUrl = $mount . '/logout.php';
$homeUrl = $mount . '/reports/index.php';
$productName = jrs_product_name();
?>
<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>AcuGIS Report Server - <?php echo jrs_html($productName); ?></title>
  <link href="https://cdnjs.cloudflare.com/ajax/libs/materialize/1.0.0/css/materialize.min.css" rel="stylesheet">
  <link href="https://fonts.googleapis.com/icon?family=Material+Icons" rel="stylesheet">
  <link href="https://fonts.googleapis.com/css2?family=Roboto:wght@300;400;500;700&display=swap" rel="stylesheet">
  <link href="portal.css?v=20260719a" rel="stylesheet">
</head>
<body data-federation-no-shell class="portal-home">
  <nav class="topnav" role="navigation">
    <div class="nav-wrapper">
      <a href="<?php echo jrs_html($homeUrl); ?>" class="brand-logo">
        <i class="material-icons brand-mark">public</i>
        <span class="brand-name">AcuGIS Report Server</span>
      </a>

      <ul class="right hide-on-med-and-down topnav-actions">
        <?php if ($isPlatformAdmin): ?>
        <li>
          <a href="<?php echo jrs_html($adminUrl); ?>" class="topnav-link">
            <i class="material-icons left">admin_panel_settings</i><span class="link-label">Administration</span>
          </a>
        </li>
        <?php endif; ?>

        <?php if ($username !== ''): ?>
        <li>
          <a href="<?php echo jrs_html($accountUrl); ?>" class="topnav-link">
            <i class="material-icons left">manage_accounts</i><span class="link-label">Account</span>
          </a>
        </li>
        <li>
          <span class="topnav-link topnav-username" title="<?php echo jrs_html($username); ?>">
            <?php echo "Welcome " . jrs_html($username); ?>          </span>
        </li>
        <?php endif; ?>
        <li>
          <a href="<?php echo jrs_html($logoutUrl); ?>" class="topnav-link">
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
    <?php echo "Welcome " . jrs_html($username); ?>
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
        <?php if ($isAdminOrEditor): ?>
        <div class="section-head-actions">
          <a href="publish_report.php"><i class="material-icons" style="font-size:18px;vertical-align:middle;">add</i> Publish report</a>
        </div>
        <?php endif; ?>
      </div>

      <div class="portal-grid" id="portal-card-grid" aria-live="polite">
        <?php if ($reports === []): ?>
          <article class="resource-card resource-card--state">
            <div class="resource-card-content resource-card-content--center">
              <div class="portal-state-icon"><i class="material-icons">assessment</i></div>
              <h3 class="resource-card-title">No reports published yet</h3>
              <p class="resource-card-desc">
                <?php if ($isAdminOrEditor): ?>
                  Publish a Jasper report to see it appear here.
                <?php else: ?>
                  Check back once reports have been published for your account.
                <?php endif; ?>
              </p>
              <?php if ($isAdminOrEditor): ?>
                <a href="publish_report.php" class="resource-open-btn" style="margin-top:12px;">Publish</a>
              <?php endif; ?>
            </div>
          </article>
        <?php else: ?>
          <?php foreach ($reports as $row): ?>
            <?php
            $report = $row['report'];
            $datasource = $row['datasource'];
            $canEdit = $isPlatformAdmin || jrs_has_permission($cconn, $uid, JRS_RESOURCE_TYPE_JASPER_REPORT, $report, 'edit');
            $canAdmin = $isPlatformAdmin || jrs_has_permission($cconn, $uid, JRS_RESOURCE_TYPE_JASPER_REPORT, $report, 'admin');
            $thumbUrl = $row['thumbnail_exists'] ? 'report_thumbnail.php?report=' . rawurlencode($report) : '';
            $viewerUrl = 'view.php?report=' . rawurlencode($report) . '&datasource=' . rawurlencode($datasource);
            $description = (string) ($row['description'] ?? '');
            $isMap = ($row['type'] ?? '') === JRS_REPORT_TYPE_MAP;
            $typeLabel = (string) ($row['type_label'] ?? 'Jasper Report');
            $badgeClass = $isMap ? 'resource-type-badge--map' : 'resource-type-badge--report';
            $previewClass = $isMap ? 'resource-preview--map' : 'resource-preview--report';
            if ($description === '') {
                $description = $isMap ? 'Map report' : 'Jasper report';
            }
            $searchBlob = strtolower($row['name'] . ' ' . $description . ' ' . $typeLabel . ' report');
            ?>
            <article class="resource-card card-type-report" data-search="<?php echo jrs_html($searchBlob); ?>">
              <?php if ($thumbUrl !== ''): ?>
                <div class="resource-preview <?php echo jrs_html($previewClass); ?>">
                  <img src="<?php echo jrs_html($thumbUrl); ?>" alt="">
                </div>
              <?php else: ?>
                <div class="resource-preview <?php echo jrs_html($previewClass); ?>" aria-hidden="true">
                  <span class="resource-preview-label"><?php echo $isMap ? 'Map Preview' : 'Report Preview'; ?></span>
                </div>
              <?php endif; ?>
              <div class="resource-card-content">
                <h3 class="resource-card-title"><?php echo jrs_html($row['name']); ?></h3>
                <p class="resource-card-desc"><?php echo jrs_html($description); ?></p>
                <div class="resource-card-footer">
                  <span class="resource-type-badge <?php echo jrs_html($badgeClass); ?>"><?php echo jrs_html($typeLabel); ?></span>
                  <div class="resource-card-actions">
                    <?php if ($canEdit): ?>
                      <a class="resource-icon-btn" href="edit_report.php?report=<?php echo rawurlencode($report); ?>" title="Edit" aria-label="Edit <?php echo jrs_html($row['name']); ?>">
                        <i class="material-icons">edit</i>
                      </a>
                    <?php endif; ?>
                    <?php if ($canAdmin): ?>
                      <a class="resource-icon-btn resource-icon-btn--danger" href="delete_report.php?report=<?php echo rawurlencode($report); ?>" title="Delete" aria-label="Delete <?php echo jrs_html($row['name']); ?>" onclick="return confirm('Delete this report?');">
                        <i class="material-icons">delete</i>
                      </a>
                    <?php endif; ?>
                    <a href="<?php echo jrs_html($viewerUrl); ?>" class="resource-open-btn" aria-label="Open <?php echo jrs_html($row['name']); ?>">Open</a>
                  </div>
                </div>
              </div>
            </article>
          <?php endforeach; ?>
        <?php endif; ?>
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
</html>
