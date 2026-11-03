<?php
function render_reports($reports) {
    if (empty($reports)) return;
?>
    <?php foreach ($reports as $row) {

        $title = htmlspecialchars($row['name'] ?? 'Report');
        $report = $row['report'] ?? '';
        $datasource = $row['datasource'] ?? '';
        $format = $row['format'] ?? 'html';
        $reportParams = $row['parameters'] ?? [];

        $viewerUrl = 'reports/view.php?report=' . rawurlencode((string) $report)
            . '&datasource=' . rawurlencode((string) $datasource);

        $image = file_exists("../assets/reports/".$report.".png")
            ? "../assets/reports/".$report.".png"
            : "";
    ?>
    <div class="group h-64 relative"
         data-type="report">
        <a href="<?=htmlspecialchars($viewerUrl, ENT_QUOTES, 'UTF-8')?>"
           class="report-card card bg-white rounded-lg overflow-hidden h-full flex flex-col"
           data-report="<?=htmlspecialchars((string) $report, ENT_QUOTES, 'UTF-8')?>"
           data-datasource="<?=htmlspecialchars((string) $datasource, ENT_QUOTES, 'UTF-8')?>"
           data-format="<?=htmlspecialchars((string) $format, ENT_QUOTES, 'UTF-8')?>"
           data-params='<?=htmlspecialchars((string) json_encode($reportParams), ENT_QUOTES, 'UTF-8')?>'>
            <div class="thumb-wrap relative">
                <?php if ($image !== '') { ?>
                    <img src="<?=$image?>?v=<?=filemtime($image)?>" class="w-full h-32 object-cover hover-zoom">
                <?php } ?>
                <div class="quick-actions">
                    <span class="px-3 py-1 bg-white/90 rounded shadow text-sm font-medium">Open Report</span>
                </div>
            </div>
            <div class="p-3 flex flex-col flex-1 justify-between">
                <div>
                    <h3 class="text-base font-semibold text-gray-900 mb-1"><?=$title?></h3>
                    <p class="text-sm text-gray-500 mb-3">Run report</p>
                </div>
            </div>
            <div class="card-foot px-3 py-2 flex items-center justify-between">
                <span class="badge badge-doc">Report</span>
                <span class="text-xs text-gray-500">Jasper</span>
            </div>
        </a>
    </div>
    <?php } ?>
<?php
}
