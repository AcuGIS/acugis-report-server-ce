<?php
declare(strict_types=1);

require_once __DIR__ . '/lib/bootstrap.php';

jrs_auth_logout();

$loginUrl = (string) jrs_config('login_url', '/acugis-report-server/login.php');
header('Location: ' . $loginUrl, true, 303);
exit;
