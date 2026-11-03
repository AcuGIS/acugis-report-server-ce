<?php
declare(strict_types=1);

/**
 * STARTTLS SMTP tests for lib/schedule_mail.php.
 *
 * Run: php tests/schedule_mail_smtp_test.php
 */

$GLOBALS['JRS_TEST_CONFIG'] = [];

function jrs_config(string $key, $default = null)
{
    return $GLOBALS['JRS_TEST_CONFIG'][$key] ?? $default;
}

require dirname(__DIR__) . '/lib/schedule_mail.php';

$failures = 0;

function jrs_test_assert(string $name, bool $ok, string $detail = ''): void
{
    global $failures;
    if ($ok) {
        fwrite(STDOUT, "ok  {$name}\n");
        return;
    }
    $failures++;
    fwrite(STDOUT, "FAIL {$name}" . ($detail !== '' ? ": {$detail}" : '') . "\n");
}

function jrs_test_make_cert(string $path): void
{
    $dn = [
        'countryName' => 'US',
        'stateOrProvinceName' => 'Test',
        'localityName' => 'Test',
        'organizationName' => 'AcuGIS Test',
        'commonName' => 'localhost',
    ];
    $privkey = openssl_pkey_new([
        'private_key_bits' => 2048,
        'private_key_type' => OPENSSL_KEYTYPE_RSA,
    ]);
    if ($privkey === false) {
        throw new RuntimeException('openssl_pkey_new failed: ' . (openssl_error_string() ?: 'unknown'));
    }
    $csr = openssl_csr_new($dn, $privkey, ['digest_alg' => 'sha256']);
    if ($csr === false) {
        throw new RuntimeException('openssl_csr_new failed: ' . (openssl_error_string() ?: 'unknown'));
    }
    $cert = openssl_csr_sign($csr, null, $privkey, 2, ['digest_alg' => 'sha256']);
    if ($cert === false) {
        throw new RuntimeException('openssl_csr_sign failed: ' . (openssl_error_string() ?: 'unknown'));
    }
    if (!openssl_x509_export($cert, $certout) || !openssl_pkey_export($privkey, $keyout)) {
        throw new RuntimeException('openssl export failed');
    }
    if (file_put_contents($path, $certout . $keyout) === false) {
        throw new RuntimeException('could not write ' . $path);
    }
}

/**
 * @return array{proc:resource, pipes:array, host:string, port:int, portFile:string, dataFile:string}
 */
function jrs_test_start_mock(string $mode, string $certFile, string $dataFile = ''): array
{
    $portFile = tempnam(sys_get_temp_dir(), 'jrsport');
    if ($portFile === false) {
        throw new RuntimeException('tempnam failed');
    }
    file_put_contents($portFile, '');
    $script = __DIR__ . DIRECTORY_SEPARATOR . 'smtp_mock_server.php';
    $cmd = [PHP_BINARY, $script, $mode, $certFile, $portFile];
    if ($dataFile !== '') {
        $cmd[] = $dataFile;
    }
    $pipes = [];
    $proc = proc_open(
        $cmd,
        [
            0 => ['pipe', 'r'],
            1 => ['pipe', 'w'],
            2 => ['pipe', 'w'],
        ],
        $pipes,
        null,
        null,
        ['bypass_shell' => true]
    );
    if (!is_resource($proc)) {
        throw new RuntimeException('proc_open failed');
    }
    stream_set_blocking($pipes[1], false);
    stream_set_blocking($pipes[2], false);

    $addr = '';
    $deadline = microtime(true) + 10;
    while (microtime(true) < $deadline) {
        $status = proc_get_status($proc);
        $addr = trim((string) file_get_contents($portFile));
        if ($addr !== '' && str_contains($addr, ':')) {
            break;
        }
        if (empty($status['running'])) {
            $err = stream_get_contents($pipes[2]) ?: 'mock exited';
            throw new RuntimeException('mock server failed: ' . $err);
        }
        usleep(20000);
    }
    if ($addr === '' || !str_contains($addr, ':')) {
        throw new RuntimeException('mock server did not publish a port');
    }
    $pos = strrpos($addr, ':');
    $host = substr($addr, 0, $pos);
    $port = (int) substr($addr, $pos + 1);
    return [
        'proc' => $proc,
        'pipes' => $pipes,
        'host' => $host !== '' ? $host : '127.0.0.1',
        'port' => $port,
        'portFile' => $portFile,
        'dataFile' => $dataFile,
    ];
}

function jrs_test_stop_mock(array $handle): void
{
    foreach ($handle['pipes'] as $pipe) {
        if (is_resource($pipe)) {
            fclose($pipe);
        }
    }
    if (isset($handle['proc']) && is_resource($handle['proc'])) {
        $status = proc_get_status($handle['proc']);
        if (!empty($status['running'])) {
            proc_terminate($handle['proc']);
        }
        proc_close($handle['proc']);
    }
    if (!empty($handle['portFile'])) {
        @unlink($handle['portFile']);
    }
}

function jrs_test_ssl_opts(): array
{
    return [
        'verify_peer' => false,
        'verify_peer_name' => false,
        'allow_self_signed' => true,
        'peer_name' => 'localhost',
    ];
}

function jrs_test_configure(array $handle, string $user, string $pass): string
{
    $attachDir = sys_get_temp_dir();
    $attach = tempnam($attachDir, 'jrsatt');
    if ($attach === false) {
        throw new RuntimeException('tempnam attach failed');
    }
    file_put_contents($attach, "%PDF-1.4 mock-report\n");
    $GLOBALS['JRS_TEST_CONFIG'] = [
        'smtp_host' => $handle['host'],
        'smtp_port' => $handle['port'],
        'smtp_user' => $user,
        'smtp_pass' => $pass,
        'smtp_from' => 'reports@example.com',
    ];
    return $attach;
}

$certFile = tempnam(sys_get_temp_dir(), 'jrscert') . '.pem';
jrs_test_make_cert($certFile);

$user = 'reports@example.com';
$pass = 'test-smtp-secret';

// STARTTLS + AUTH LOGIN success, including MIME attachment.
$dataFile = tempnam(sys_get_temp_dir(), 'jrsdata');
file_put_contents($dataFile, '');
$handle = jrs_test_start_mock('success', $certFile, $dataFile);
try {
    $attach = jrs_test_configure($handle, $user, $pass);
    $result = jrs_schedule_send_smtp(
        ['dest@example.com'],
        'Scheduled report',
        'Body text',
        $attach,
        'report.pdf',
        false,
        jrs_test_ssl_opts()
    );
    jrs_test_assert('STARTTLS + AUTH LOGIN success', !empty($result['ok']) && ($result['message'] ?? '') === 'sent', json_encode($result));
    $payload = (string) file_get_contents($dataFile);
    jrs_test_assert(
        'MIME attachment preserved',
        str_contains($payload, 'Content-Type: multipart/mixed')
        && str_contains($payload, 'filename="report.pdf"')
        && str_contains($payload, 'Body text')
        && str_contains($payload, chunk_split(base64_encode((string) file_get_contents($attach)))),
        substr($payload, 0, 200)
    );
    @unlink($attach);
} finally {
    jrs_test_stop_mock($handle);
    @unlink($dataFile);
}

// STARTTLS rejection.
$handle = jrs_test_start_mock('starttls-reject', $certFile);
try {
    $attach = jrs_test_configure($handle, $user, $pass);
    $result = jrs_schedule_send_smtp(
        ['dest@example.com'],
        'subj',
        'body',
        $attach,
        'report.pdf',
        false,
        jrs_test_ssl_opts()
    );
    $msg = (string) ($result['message'] ?? '');
    jrs_test_assert(
        'STARTTLS rejection',
        empty($result['ok']) && str_starts_with($msg, 'SMTP STARTTLS rejected:') && str_contains($msg, '454'),
        $msg
    );
    jrs_test_assert('STARTTLS rejection does not leak password', !str_contains($msg, $pass), $msg);
    @unlink($attach);
} finally {
    jrs_test_stop_mock($handle);
}

// TLS negotiation failure.
$handle = jrs_test_start_mock('tls-fail', $certFile);
try {
    $attach = jrs_test_configure($handle, $user, $pass);
    $result = jrs_schedule_send_smtp(
        ['dest@example.com'],
        'subj',
        'body',
        $attach,
        'report.pdf',
        false,
        jrs_test_ssl_opts()
    );
    $msg = (string) ($result['message'] ?? '');
    jrs_test_assert(
        'TLS negotiation failure',
        empty($result['ok']) && $msg === 'SMTP TLS handshake failed',
        $msg
    );
    @unlink($attach);
} finally {
    jrs_test_stop_mock($handle);
}

// Authentication rejection after STARTTLS.
$handle = jrs_test_start_mock('auth-reject', $certFile);
try {
    $attach = jrs_test_configure($handle, $user, $pass);
    $result = jrs_schedule_send_smtp(
        ['dest@example.com'],
        'subj',
        'body',
        $attach,
        'report.pdf',
        false,
        jrs_test_ssl_opts()
    );
    $msg = (string) ($result['message'] ?? '');
    jrs_test_assert(
        'authentication rejection',
        empty($result['ok']) && str_starts_with($msg, 'SMTP authentication rejected:') && str_contains($msg, '535'),
        $msg
    );
    jrs_test_assert('authentication rejection does not leak password', !str_contains($msg, $pass) && !str_contains($msg, base64_encode($pass)), $msg);
    @unlink($attach);
} finally {
    jrs_test_stop_mock($handle);
}

// Port 587 without STARTTLS capability (session sees submission port even
// when the mock is bound to an ephemeral TCP port).
$handle = jrs_test_start_mock('no-starttls', $certFile);
$fp = null;
try {
    $attach = jrs_test_configure($handle, $user, $pass);
    $remote = 'tcp://' . $handle['host'] . ':' . $handle['port'];
    $fp = stream_socket_client($remote, $errno, $errstr, 20);
    if ($fp === false) {
        throw new RuntimeException('connect to mock failed: ' . $errstr);
    }
    $result = jrs_schedule_smtp_session(
        $fp,
        $handle['host'],
        587,
        $user,
        $pass,
        'reports@example.com',
        ['dest@example.com'],
        'subj',
        'body',
        $attach,
        'report.pdf',
        false
    );
    $msg = (string) ($result['message'] ?? '');
    jrs_test_assert(
        'STARTTLS not supported on 587',
        empty($result['ok']) && $msg === 'SMTP STARTTLS not supported',
        $msg
    );
    @unlink($attach);
} finally {
    if (is_resource($fp)) {
        fclose($fp);
    }
    jrs_test_stop_mock($handle);
}

@unlink($certFile);

if ($failures > 0) {
    fwrite(STDOUT, "{$failures} test(s) failed\n");
    exit(1);
}
fwrite(STDOUT, "all tests passed\n");
exit(0);
