<?php
declare(strict_types=1);

/**
 * Email templates from the legacy email_tmpl directory (HTML files on disk).
 * Templates are not created via the Jasper portal — add .html files on the server.
 */

function jrs_email_tmpl_dir(): string
{
    $dir = trim((string) jrs_config('jasper_email_tmpl_dir', ''));
    if ($dir !== '') {
        return rtrim($dir, '/\\');
    }
    return jrs_jasper_home() . '/email_tmpl';
}

/**
 * @return list<string> Template filenames for the schedule dropdown (e.g. email_template.html)
 */
function jrs_email_tmpl_list(): array
{
    $dir = jrs_email_tmpl_dir();
    if (!is_dir($dir)) {
        return [];
    }

    $names = [];
    foreach (scandir($dir) ?: [] as $entry) {
        if ($entry === '.' || $entry === '..') {
            continue;
        }
        $path = $dir . DIRECTORY_SEPARATOR . $entry;
        if (!is_file($path)) {
            continue;
        }
        if (!preg_match('/\.(html?|txt)$/i', $entry)) {
            continue;
        }
        $names[] = $entry;
    }

    sort($names, SORT_NATURAL | SORT_FLAG_CASE);
    return $names;
}

/**
 * Resolve a template name to a readable file path (no path traversal).
 */
function jrs_email_tmpl_resolve_path(string $name): ?string
{
    $name = trim($name);
    if ($name === '') {
        return null;
    }
    $name = basename(str_replace(['\\', '/'], '', $name));
    if ($name === '') {
        return null;
    }

    $dir = jrs_email_tmpl_dir();
    $path = $dir . DIRECTORY_SEPARATOR . $name;
    if (is_file($path)) {
        return $path;
    }
    if (!preg_match('/\./', $name)) {
        foreach (['.html', '.htm', '.txt'] as $ext) {
            $try = $dir . DIRECTORY_SEPARATOR . $name . $ext;
            if (is_file($try)) {
                return $try;
            }
        }
    }

    return null;
}

/**
 * Load template file contents (HTML body). Returns null when missing.
 */
function jrs_email_tmpl_load(string $name): ?string
{
    $path = jrs_email_tmpl_resolve_path($name);
    if ($path === null) {
        return null;
    }
    $contents = @file_get_contents($path);
    if ($contents === false || $contents === '') {
        return null;
    }
    return $contents;
}

function jrs_email_tmpl_is_html(string $name, string $contents): bool
{
    if (preg_match('/\.html?$/i', $name) === 1) {
        return true;
    }
    $trim = ltrim($contents);
    return stripos($trim, '<!doctype') === 0
        || stripos($trim, '<html') === 0
        || stripos($trim, '<body') === 0;
}

function jrs_email_apply_placeholders(string $text, array $vars): string
{
    foreach ($vars as $key => $value) {
        $text = str_replace('{{' . $key . '}}', (string) $value, $text);
    }
    return $text;
}
