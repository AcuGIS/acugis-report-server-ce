<?php
declare(strict_types=1);

require_once __DIR__ . '/lib/bootstrap.php';

$cconn = jrs_auth_require_central_db();
jrs_require_login($cconn);
jrs_require_service_access($cconn, 'jasper_reports', 'view');
jrs_require_schedule_admin($cconn);

$conn = jrs_app_db();
$sched = [
    'cron_period' => 'daily',
    'schedule_mode' => 'daily',
    'cron_custom' => '*/10 * * * *',
    'repeat_minutes' => 10,
    'hourly_minute' => 0,
    'daily_time' => '08:00',
    'weekly_day' => 'Monday',
    'weekly_time' => '08:00',
    'monthly_day' => 1,
    'monthly_time' => '08:00',
    'once_datetime' => '',
    'report_name' => '',
    'datasource_name' => '',
    'format' => 'pdf',
    'filename' => '',
    'email' => '',
    'email_subject' => '',
    'email_body' => '',
    'email_template' => '',
    'noemail' => false,
    'url_opt_params' => '',
    'enabled' => true,
];
$editId = isset($_GET['id']) ? (int) $_GET['id'] : 0;
$error = isset($_GET['error']) ? (string) $_GET['error'] : '';

if ($conn && $editId > 0) {
    $loaded = jrs_schedule_get($conn, $editId);
    if ($loaded === null) {
        $error = 'Schedule not found.';
    } else {
        $sched = $loaded;
    }
}

$reports = jrs_all_reports(false);
$datasources = $conn ? jrs_datasources($conn) : [];
$emlTmpls = jrs_email_tmpl_list();
$tmplDir = jrs_email_tmpl_dir();
$recipients = $conn ? jrs_recipient_list($conn) : [];
$selectedTmpl = trim((string) ($sched['email_template'] ?? ''));
$formSchedule = jrs_schedule_form_field_defaults($sched);
$scheduleMode = (string) ($formSchedule['schedule_mode'] ?? 'daily');
$scheduleSummary = jrs_schedule_summary(array_merge($sched, $formSchedule));
$scheduleEnabled = jrs_schedule_is_enabled($sched);
if ($editId > 0 && !$scheduleEnabled) {
    $cron = strtolower(trim((string) ($sched['cron_period'] ?? 'never')));
    if ($cron !== 'never' && $cron !== 'now') {
        $scheduleEnabled = true;
    }
}

$pageTitle = $editId > 0 ? 'Edit schedule #' . $editId : 'Add schedule';
jrs_material_page_header($pageTitle);
?>
<header class="page-header">
    <div>
        <h1 class="page-title"><?php echo jrs_html($pageTitle); ?></h1>
        <p class="page-subtitle">Configure report execution, timing, and email delivery.</p>
    </div>
    <div class="header-actions">
        <a href="schedules.php" class="btn btn-flat waves-effect">
            <i class="material-icons left">arrow_back</i>Back
        </a>
    </div>
</header>

<div class="card">
    <div class="card-content">
        <?php if ($error !== ''): ?>
            <div class="card-panel red lighten-4"><?php echo jrs_html($error); ?></div>
        <?php endif; ?>

        <form id="sched_form" method="post" action="#">
            <?php if ($editId > 0): ?>
                <input type="hidden" name="id" value="<?php echo $editId; ?>">
            <?php endif; ?>

            <fieldset class="stack-sm" id="schedule_fieldset">
                <legend>Run</legend>
                <p class="muted-small" id="schedule_summary"><?php echo jrs_html($scheduleSummary); ?></p>

                <div class="row schedule-mode-row">
                    <?php
                    $modeLabels = [
                        'once' => 'Once',
                        'repeat_minutes' => 'Every N Minutes',
                        'hourly' => 'Hourly',
                        'daily' => 'Daily',
                        'weekly' => 'Weekly',
                        'monthly' => 'Monthly',
                        'custom' => 'Custom (Advanced)',
                    ];
                    foreach (jrs_schedule_modes() as $mode):
                        $mid = 'schedule_mode_' . $mode;
                    ?>
                    <p class="col s12 m6 l4">
                        <label>
                            <input class="schedule-mode-radio" type="radio" name="schedule_mode" id="<?php echo jrs_html($mid); ?>"
                                value="<?php echo jrs_html($mode); ?>" <?php echo $scheduleMode === $mode ? 'checked' : ''; ?>>
                            <span><?php echo jrs_html($modeLabels[$mode] ?? $mode); ?></span>
                        </label>
                    </p>
                    <?php endforeach; ?>
                </div>

                <div id="panel_repeat_minutes" class="schedule-panel row <?php echo $scheduleMode !== 'repeat_minutes' ? 'is-hidden' : ''; ?>">
                    <div class="col s12">
                        <span>Every</span>
                        <div class="input-field inline" style="width: 5rem; margin: 0 0.5rem;">
                            <input type="number" name="repeat_minutes" id="repeat_minutes" min="1" max="1440"
                                value="<?php echo (int) ($formSchedule['repeat_minutes'] ?? 10); ?>">
                        </div>
                        <span>minutes</span>
                    </div>
                </div>

                <div id="panel_hourly" class="schedule-panel row <?php echo $scheduleMode !== 'hourly' ? 'is-hidden' : ''; ?>">
                    <div class="col s12">
                        <span>Every hour at minute</span>
                        <div class="input-field inline" style="width: 4.5rem; margin: 0 0.5rem;">
                            <input type="number" name="hourly_minute" id="hourly_minute" min="0" max="59"
                                value="<?php echo (int) ($formSchedule['hourly_minute'] ?? 0); ?>">
                        </div>
                    </div>
                </div>

                <div id="panel_daily" class="schedule-panel row <?php echo $scheduleMode !== 'daily' ? 'is-hidden' : ''; ?>">
                    <div class="col s12 m6">
                        <span>Every day at</span>
                        <div class="input-field inline" style="width: 8rem; margin-left: 0.5rem;">
                            <input type="time" name="daily_time" id="daily_time"
                                value="<?php echo jrs_html((string) ($formSchedule['daily_time'] ?? '08:00')); ?>">
                        </div>
                    </div>
                </div>

                <div id="panel_weekly" class="schedule-panel row <?php echo $scheduleMode !== 'weekly' ? 'is-hidden' : ''; ?>">
                    <div class="input-field col s12 m6">
                        <select name="weekly_day" id="weekly_day">
                            <?php foreach (jrs_schedule_weekly_days() as $day): ?>
                                <option value="<?php echo jrs_html($day); ?>" <?php echo ($formSchedule['weekly_day'] ?? '') === $day ? 'selected' : ''; ?>>
                                    <?php echo jrs_html($day); ?>
                                </option>
                            <?php endforeach; ?>
                        </select>
                        <label>Day</label>
                    </div>
                    <div class="col s12 m6">
                        <span>at</span>
                        <div class="input-field inline" style="width: 8rem; margin-left: 0.5rem;">
                            <input type="time" name="weekly_time" id="weekly_time"
                                value="<?php echo jrs_html((string) ($formSchedule['weekly_time'] ?? '08:00')); ?>">
                        </div>
                    </div>
                </div>

                <div id="panel_monthly" class="schedule-panel row <?php echo $scheduleMode !== 'monthly' ? 'is-hidden' : ''; ?>">
                    <div class="col s12">
                        <span>Day</span>
                        <div class="input-field inline" style="width: 4rem; margin: 0 0.5rem;">
                            <input type="number" name="monthly_day" id="monthly_day" min="1" max="31"
                                value="<?php echo (int) ($formSchedule['monthly_day'] ?? 1); ?>">
                        </div>
                        <span>at</span>
                        <div class="input-field inline" style="width: 8rem; margin: 0 0.5rem;">
                            <input type="time" name="monthly_time" id="monthly_time"
                                value="<?php echo jrs_html((string) ($formSchedule['monthly_time'] ?? '08:00')); ?>">
                        </div>
                    </div>
                </div>

                <div id="panel_once" class="schedule-panel row <?php echo $scheduleMode !== 'once' ? 'is-hidden' : ''; ?>">
                    <div class="col s12 m8">
                        <span>Run at</span>
                        <div class="input-field inline" style="width: 14rem; margin-left: 0.5rem;">
                            <input type="datetime-local" name="once_datetime" id="once_datetime"
                                value="<?php echo jrs_html((string) ($formSchedule['once_datetime'] ?? '')); ?>">
                        </div>
                    </div>
                </div>

                <div id="panel_custom" class="schedule-panel <?php echo $scheduleMode !== 'custom' ? 'is-hidden' : ''; ?>">
                    <div class="input-field">
                        <input type="text" name="cron_custom" id="cron_custom"
                            value="<?php echo jrs_html((string) ($formSchedule['cron_custom'] ?? '*/10 * * * *')); ?>">
                        <label for="cron_custom" class="active">Cron expression</label>
                    </div>
                    <p class="muted-small">Examples: <code>*/10 * * * *</code>, <code>0 * * * *</code>, <code>15 2 * * *</code></p>
                </div>

                <p class="col s12" style="margin-top: 1rem;">
                    <label>
                        <input type="checkbox" id="schedule_enabled" value="1" <?php echo $scheduleEnabled ? 'checked' : ''; ?>>
                        <span>Enable schedule</span>
                    </label>
                    <span class="muted-small" style="display: block; margin-left: 35px;">
                        When enabled, a systemd timer is installed for this schedule.
                    </span>
                </p>
            </fieldset>

            <div class="row">
                <div class="input-field col s12 m6">
                    <select name="report_name" id="report_name" required>
                        <option value="" disabled <?php echo ($sched['report_name'] ?? '') === '' ? 'selected' : ''; ?>>Select report</option>
                        <?php foreach ($reports as $rep): ?>
                            <?php $key = (string) ($rep['report'] ?? ''); ?>
                            <option value="<?php echo jrs_html($key); ?>" data-datasource="<?php echo jrs_html((string) ($rep['datasource'] ?? '')); ?>"
                                <?php echo ($sched['report_name'] ?? '') === $key ? 'selected' : ''; ?>>
                                <?php echo jrs_html((string) ($rep['name'] ?? $key)); ?> (<?php echo jrs_html($key); ?>)
                            </option>
                        <?php endforeach; ?>
                    </select>
                    <label>Jasper report</label>
                </div>
                <div class="input-field col s12 m6">
                    <select name="datasource_name" id="datasource_name" required>
                        <?php foreach ($datasources as $ds): ?>
                            <?php $dn = (string) ($ds['name'] ?? ''); ?>
                            <option value="<?php echo jrs_html($dn); ?>" <?php echo ($sched['datasource_name'] ?? '') === $dn ? 'selected' : ''; ?>>
                                <?php echo jrs_html($dn); ?>
                            </option>
                        <?php endforeach; ?>
                    </select>
                    <label>Datasource</label>
                </div>
            </div>

            <div class="row">
                <div class="input-field col s12 m4">
                    <select name="format" id="format">
                        <?php foreach (jrs_schedule_output_formats() as $fmt): ?>
                            <option value="<?php echo jrs_html($fmt); ?>" <?php echo ($sched['format'] ?? '') === $fmt ? 'selected' : ''; ?>>
                                <?php echo jrs_html($fmt); ?>
                            </option>
                        <?php endforeach; ?>
                    </select>
                    <label>Output format</label>
                </div>
                <div class="input-field col s12 m8">
                    <input type="text" name="filename" id="filename" value="<?php echo jrs_html((string) ($sched['filename'] ?? '')); ?>" required>
                    <label for="filename" class="active">Output filename (without extension)</label>
                </div>
            </div>

            <div class="row">
                <div class="input-field col s12 m8">
                    <input type="text" name="email" id="email" value="<?php echo jrs_html((string) ($sched['email'] ?? '')); ?>">
                    <label for="email" class="active">Email recipients (comma-separated)</label>
                </div>
                <div class="col s12 m4" style="padding-top: 1.2rem;">
                    <label>
                        <input type="checkbox" name="noemail" id="noemail" value="t" <?php echo !empty($sched['noemail']) ? 'checked' : ''; ?>>
                        <span>Don't send email</span>
                    </label>
                </div>
            </div>

            <?php if ($recipients !== []): ?>
            <div class="row">
                <div class="input-field col s12 m6">
                    <select id="recipient_picker">
                        <option value="">Add recipient…</option>
                        <?php foreach ($recipients as $rc): ?>
                            <option value="<?php echo jrs_html((string) $rc['email']); ?>">
                                <?php echo jrs_html((string) ($rc['name'] !== '' ? $rc['name'] : $rc['email'])); ?>
                            </option>
                        <?php endforeach; ?>
                    </select>
                    <label>Recipients directory</label>
                </div>
            </div>
            <?php endif; ?>

            <fieldset class="stack-sm" id="email_fields">
                <legend>Optional email fields</legend>
                <p class="muted-small">Placeholders: <code>{{report_name}}</code>, <code>{{filename}}</code>, <code>{{run_date}}</code>, <code>{{format}}</code></p>

                <div class="input-field">
                    <input type="text" name="email_subject" id="email_subject" value="<?php echo jrs_html((string) ($sched['email_subject'] ?? '')); ?>">
                    <label for="email_subject" class="active">Email subject</label>
                </div>

                <div class="input-field">
                    <textarea name="email_body" id="email_body" class="materialize-textarea"><?php echo jrs_html((string) ($sched['email_body'] ?? '')); ?></textarea>
                    <label for="email_body" class="active">Email body</label>
                </div>

                <div class="input-field">
                    <select name="email_tmpl" id="email_tmpl">
                        <option value=""></option>
                        <?php foreach ($emlTmpls as $tmplName): ?>
                            <option value="<?php echo jrs_html($tmplName); ?>" <?php echo $selectedTmpl === $tmplName ? 'selected' : ''; ?>>
                                <?php echo jrs_html($tmplName); ?>
                            </option>
                        <?php endforeach; ?>
                    </select>
                    <label>Email template</label>
                </div>
                <p class="muted-small">
                    Templates are HTML files in <code><?php echo jrs_html($tmplDir); ?></code>
                    (add or edit files on the server; not managed in this UI).
                </p>
            </fieldset>

            <div class="input-field">
                <textarea name="url_opt_params" id="url_opt_params" class="materialize-textarea"><?php echo jrs_html((string) ($sched['url_opt_params'] ?? '')); ?></textarea>
                <label for="url_opt_params" class="active">Optional report parameters (key=value, comma-separated)</label>
            </div>

            <div class="form-actions">
                <button type="button" class="btn waves-effect waves-light teal" id="btn_submit">
                    <?php echo $editId > 0 ? 'Update schedule' : 'Create schedule'; ?>
                </button>
            </div>
        </form>
    </div>
</div>

<script>
document.addEventListener('DOMContentLoaded', function () {
    var modeRadios = document.querySelectorAll('.schedule-mode-radio');
    var summaryEl = document.getElementById('schedule_summary');
    var reportSel = document.getElementById('report_name');
    var dsSel = document.getElementById('datasource_name');
    var noemail = document.getElementById('noemail');
    var scheduleEnabled = document.getElementById('schedule_enabled');
    var emailFields = document.getElementById('email_fields');
    var emailInput = document.getElementById('email');
    var recipientPicker = document.getElementById('recipient_picker');

    var panelIds = {
        once: 'panel_once',
        repeat_minutes: 'panel_repeat_minutes',
        hourly: 'panel_hourly',
        daily: 'panel_daily',
        weekly: 'panel_weekly',
        monthly: 'panel_monthly',
        custom: 'panel_custom'
    };

    function selectedMode() {
        var checked = document.querySelector('.schedule-mode-radio:checked');
        return checked ? checked.value : 'daily';
    }

    function toggleSchedulePanels() {
        var mode = selectedMode();
        Object.keys(panelIds).forEach(function (key) {
            var el = document.getElementById(panelIds[key]);
            if (!el) return;
            el.classList.toggle('is-hidden', key !== mode);
        });
        updateScheduleSummary();
    }

    function pad2(n) { return (n < 10 ? '0' : '') + n; }

    function updateScheduleSummary() {
        var mode = selectedMode();
        var text = '';
        if (mode === 'repeat_minutes') {
            var n = parseInt(document.getElementById('repeat_minutes').value, 10) || 10;
            text = 'Runs every ' + n + ' minute' + (n === 1 ? '' : 's');
        } else if (mode === 'hourly') {
            var m = parseInt(document.getElementById('hourly_minute').value, 10) || 0;
            text = 'Runs hourly at minute ' + pad2(m);
        } else if (mode === 'daily') {
            text = 'Runs daily at ' + (document.getElementById('daily_time').value || '08:00');
        } else if (mode === 'weekly') {
            var daySel = document.getElementById('weekly_day');
            var day = daySel.options[daySel.selectedIndex] ? daySel.options[daySel.selectedIndex].text : 'Monday';
            text = 'Runs ' + day + 's at ' + (document.getElementById('weekly_time').value || '08:00');
        } else if (mode === 'monthly') {
            var dom = parseInt(document.getElementById('monthly_day').value, 10) || 1;
            text = 'Runs monthly on day ' + dom + ' at ' + (document.getElementById('monthly_time').value || '08:00');
        } else if (mode === 'once') {
            var dt = document.getElementById('once_datetime').value;
            text = dt ? ('Runs once at ' + dt.replace('T', ' ')) : 'Runs once at the selected date and time';
        } else if (mode === 'custom') {
            text = 'Custom: ' + (document.getElementById('cron_custom').value || '');
        }
        if (summaryEl) summaryEl.textContent = text;
    }

    function toggleEmailFields() {
        var off = noemail.checked;
        emailFields.style.display = off ? 'none' : '';
        emailInput.required = !off;
    }
    function syncDatasourceFromReport() {
        var opt = reportSel.options[reportSel.selectedIndex];
        if (!opt) return;
        var ds = opt.getAttribute('data-datasource');
        if (!ds) return;
        for (var i = 0; i < dsSel.options.length; i++) {
            if (dsSel.options[i].value === ds) {
                dsSel.selectedIndex = i;
                break;
            }
        }
        if (window.M) M.FormSelect.init(dsSel);
    }

    modeRadios.forEach(function (radio) {
        radio.addEventListener('change', toggleSchedulePanels);
    });
    ['repeat_minutes', 'hourly_minute', 'daily_time', 'weekly_day', 'weekly_time', 'monthly_day', 'monthly_time', 'once_datetime', 'cron_custom'].forEach(function (id) {
        var el = document.getElementById(id);
        if (el) el.addEventListener('input', updateScheduleSummary);
        if (el) el.addEventListener('change', updateScheduleSummary);
    });
    noemail.addEventListener('change', toggleEmailFields);
    reportSel.addEventListener('change', syncDatasourceFromReport);
    if (recipientPicker) {
        recipientPicker.addEventListener('change', function () {
            var v = recipientPicker.value;
            if (!v) return;
            var cur = emailInput.value.trim();
            emailInput.value = cur ? (cur + ', ' + v) : v;
            recipientPicker.selectedIndex = 0;
            if (window.M) M.updateTextFields();
        });
    }

    toggleSchedulePanels();
    toggleEmailFields();

    function validateScheduleClient() {
        var mode = selectedMode();
        if (mode === 'repeat_minutes') {
            var n = parseInt(document.getElementById('repeat_minutes').value, 10);
            if (isNaN(n) || n < 1 || n > 1440) return 'Repeat interval must be between 1 and 1440 minutes.';
        } else if (mode === 'hourly') {
            var m = parseInt(document.getElementById('hourly_minute').value, 10);
            if (isNaN(m) || m < 0 || m > 59) return 'Hourly minute must be between 0 and 59.';
        } else if (mode === 'monthly') {
            var dom = parseInt(document.getElementById('monthly_day').value, 10);
            if (isNaN(dom) || dom < 1 || dom > 31) return 'Monthly day must be between 1 and 31.';
        } else if (mode === 'once') {
            if (!document.getElementById('once_datetime').value) return 'Select a date and time for the one-time run.';
        } else if (mode === 'custom') {
            var cron = document.getElementById('cron_custom').value.trim();
            if (!cron) return 'Cron expression is required.';
            if (cron.split(/\s+/).length !== 5) return 'Cron expression must have five fields.';
        }
        return '';
    }

    document.getElementById('btn_submit').addEventListener('click', function () {
        var schedErr = validateScheduleClient();
        if (schedErr) {
            alert(schedErr);
            return;
        }
        var form = document.getElementById('sched_form');
        var data = new FormData(form);
        data.append(<?php echo $editId > 0 ? "'update'" : "'create'"; ?>, '1');
        if (noemail.checked) data.set('noemail', 't');
        data.set('enabled', scheduleEnabled && scheduleEnabled.checked ? '1' : '0');

        fetch('action/schedule.php', {
            method: 'POST',
            body: data,
            credentials: 'same-origin'
        }).then(function (r) {
            return r.text().then(function (text) {
                if (!r.ok) {
                    throw new Error('HTTP ' + r.status + (text ? ': ' + text.slice(0, 500) : ''));
                }
                try {
                    return JSON.parse(text);
                } catch (e) {
                    throw new Error(text ? text.slice(0, 500) : 'Invalid JSON from server');
                }
            });
        }).then(function (res) {
            alert(res.message || (res.success ? 'Saved' : 'Failed'));
            if (res.success) window.location.href = 'schedules.php';
        }).catch(function (err) {
            alert(err.message || String(err));
        });
    });
});
</script>
<?php jrs_material_page_footer(); ?>
