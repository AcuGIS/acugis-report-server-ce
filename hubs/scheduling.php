<?php
declare(strict_types=1);

require_once __DIR__ . '/../lib/bootstrap.php';

$hubId = 'scheduling';
$title = 'Scheduling';
$safeHub = jrs_html($hubId);
$safeTitle = jrs_html($title);
$product = jrs_html(jrs_product_name());

echo '<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="utf-8">
    <meta name="viewport" content="width=device-width, initial-scale=1">
    <title>' . $safeTitle . ' - ' . $product . '</title>
    <link href="https://cdnjs.cloudflare.com/ajax/libs/materialize/1.0.0/css/materialize.min.css" rel="stylesheet">
    <link href="https://fonts.googleapis.com/icon?family=Material+Icons" rel="stylesheet">
    <link href="https://fonts.googleapis.com/css2?family=Roboto:wght@300;400;500;700&display=swap" rel="stylesheet">
    ' . jrs_shell_bootstrap_tag() . '
</head>
<body data-federation-app="jasper" data-federation-hub="' . $safeHub . '" class="fed-shell-body">
' . jrs_shell_chrome_html() . '
<main class="fed-shell-main" data-federation-hub="' . $safeHub . '"></main>
</body>
</html>';
