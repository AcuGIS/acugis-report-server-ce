<?php
declare(strict_types=1);

require_once __DIR__ . '/lib/bootstrap.php';

$productName = (string) jrs_config('product_name', 'AcuGIS Report Server');
$appMount = (string) jrs_config('app_mount', '/acugis-report-server');
$defaultNext = rtrim($appMount, '/') . '/reports/index.php';
$error = '';

function jrs_login_safe_next(string $next, string $defaultNext): string
{
    $next = trim($next);
    if ($next === '' || !str_starts_with($next, '/') || str_starts_with($next, '//')) {
        return $defaultNext;
    }
    return $next;
}

$next = jrs_login_safe_next((string) ($_GET['next'] ?? ''), $defaultNext);

if (jrs_auth_is_enabled()) {
    $conn = jrs_app_db();
    if ($conn) {
        jrs_auth_try_refresh_current_user($conn);
        $user = jrs_get_current_user();
        if ((int) ($user['id'] ?? 0) > 0) {
            header('Location: ' . jrs_login_safe_next((string) ($_POST['next'] ?? $next), $defaultNext), true, 303);
            exit;
        }
    }
} elseif ((int) (jrs_get_current_user()['id'] ?? 0) <= 0) {
    $GLOBALS['JRS_AUTH_USER'] = [
        'id' => 0,
        'username' => 'local-dev',
        'email' => '',
        'is_admin' => true,
        'groups' => [],
    ];
    header('Location: ' . $next, true, 303);
    exit;
}

if ($_SERVER['REQUEST_METHOD'] === 'POST') {
    $username = trim((string) ($_POST['username'] ?? ''));
    $password = (string) ($_POST['password'] ?? '');
    $next = jrs_login_safe_next((string) ($_POST['next'] ?? $next), $defaultNext);

    if ($username === '' || $password === '') {
        $error = 'Username and password are required.';
    } elseif (jrs_auth_login($username, $password)) {
        header('Location: ' . $next, true, 303);
        exit;
    } else {
        $error = 'Invalid username or password.';
    }
}

$safeTitle = jrs_html($productName);
$safeError = jrs_html($error);
$safeNext = jrs_html($next);
?>
<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="utf-8">
    <meta name="viewport" content="width=device-width, initial-scale=1">
    <title>Sign in - <?php echo $safeTitle; ?></title>
    <link href="https://cdnjs.cloudflare.com/ajax/libs/materialize/1.0.0/css/materialize.min.css" rel="stylesheet">
    <link href="https://fonts.googleapis.com/icon?family=Material+Icons" rel="stylesheet">
    <link href="https://fonts.googleapis.com/css2?family=Roboto:wght@300;400;500;700&display=swap" rel="stylesheet">
    <style>
        body {
            background: #f5f5f5;
            min-height: 100vh;
            display: flex;
            align-items: center;
            justify-content: center;
            font-family: Roboto, sans-serif;
        }
        .login-card {
            width: 100%;
            max-width: 420px;
            margin: 24px;
            border-radius: 8px;
        }
        .login-brand {
            font-size: 1.35rem;
            font-weight: 500;
            color: #263238;
            margin-bottom: 8px;
        }
        .login-subtitle {
            color: #607d8b;
            margin-bottom: 24px;
        }
        .error-banner {
            background: #ffebee;
            color: #c62828;
            border-radius: 4px;
            padding: 12px 14px;
            margin-bottom: 20px;
        }
    </style>
</head>
<body>
<div class="card login-card">
    <div class="card-content">
        <div class="login-brand"><?php echo $safeTitle; ?></div>
        <p class="login-subtitle">Sign in to manage reports and schedules.</p>

        <?php if ($error !== ''): ?>
            <div class="error-banner"><?php echo $safeError; ?></div>
        <?php endif; ?>

        <form method="post" action="">
            <input type="hidden" name="next" value="<?php echo $safeNext; ?>">
            <div class="input-field">
                <input id="username" name="username" type="text" required autocomplete="username" autofocus>
                <label for="username">Username</label>
            </div>
            <div class="input-field">
                <input id="password" name="password" type="password" required autocomplete="current-password">
                <label for="password">Password</label>
            </div>
            <button type="submit" class="btn waves-effect waves-light teal darken-1" style="width:100%;">
                Sign in
            </button>
        </form>
    </div>
</div>
<script src="https://cdnjs.cloudflare.com/ajax/libs/materialize/1.0.0/js/materialize.min.js"></script>
<script>
document.addEventListener('DOMContentLoaded', function () {
    if (window.M) {
        M.updateTextFields();
    }
});
</script>
</body>
</html>
