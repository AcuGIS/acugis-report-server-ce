<?php
declare(strict_types=1);

/**
 * Local SMTP mock for scheduled-mail STARTTLS tests.
 *
 * Usage: php smtp_mock_server.php <mode> <cert.pem> <portfile> [datafile]
 *
 * Modes:
 *   success          STARTTLS + AUTH LOGIN + accept mail
 *   starttls-reject  reject STARTTLS with 454
 *   tls-fail         220 STARTTLS then close without TLS
 *   auth-reject      STARTTLS then reject AUTH LOGIN password
 *   no-starttls      EHLO without STARTTLS (port 587 client must fail)
 */
$mode = $argv[1] ?? 'success';
$certFile = $argv[2] ?? '';
$portFile = $argv[3] ?? '';
$dataFile = $argv[4] ?? '';

$ssl = [
    'local_cert' => $certFile,
    'allow_self_signed' => true,
    'verify_peer' => false,
    'verify_peer_name' => false,
    'SNI_enabled' => true,
];
if (defined('STREAM_CRYPTO_METHOD_TLSv1_2_SERVER')) {
    $method = STREAM_CRYPTO_METHOD_TLSv1_2_SERVER;
    if (defined('STREAM_CRYPTO_METHOD_TLSv1_3_SERVER')) {
        $method |= STREAM_CRYPTO_METHOD_TLSv1_3_SERVER;
    }
    $ssl['crypto_method'] = $method;
}
$ctx = stream_context_create(['ssl' => $ssl]);
$server = @stream_socket_server('tcp://127.0.0.1:0', $errno, $errstr, STREAM_SERVER_BIND | STREAM_SERVER_LISTEN, $ctx);
if (!$server) {
    fwrite(STDERR, "bind failed: {$errstr}\n");
    exit(1);
}
$addr = stream_socket_get_name($server, false);
if ($portFile !== '') {
    file_put_contents($portFile, $addr);
}

$client = @stream_socket_accept($server, 20);
if (!$client) {
    exit(1);
}
stream_set_timeout($client, 20);

$tls = false;

$write = static function (string $line) use ($client): void {
    fwrite($client, $line . "\r\n");
};

$readLine = static function () use ($client): ?string {
    $line = fgets($client, 8192);
    if ($line === false) {
        return null;
    }
    return rtrim($line, "\r\n");
};

$write('220 mock.example.com ESMTP');

$enableTls = static function () use ($client, $mode): bool {
    if ($mode === 'tls-fail') {
        fclose($client);
        return false;
    }
    $method = STREAM_CRYPTO_METHOD_TLSv1_2_SERVER;
    if (defined('STREAM_CRYPTO_METHOD_TLSv1_3_SERVER')) {
        $method |= STREAM_CRYPTO_METHOD_TLSv1_3_SERVER;
    }
    $ok = @stream_socket_enable_crypto($client, true, $method);
    $deadline = microtime(true) + 20;
    while ($ok === 0 && microtime(true) < $deadline) {
        $ok = @stream_socket_enable_crypto($client, true, $method);
    }
    return $ok === true;
};

$ehloPlain = static function () use ($write, $mode): void {
    $write('250-mock.example.com');
    if ($mode !== 'no-starttls') {
        $write('250-STARTTLS');
    }
    $write('250 PIPELINING');
};

$ehloTls = static function () use ($write): void {
    $write('250-mock.example.com');
    $write('250-AUTH LOGIN PLAIN');
    $write('250 PIPELINING');
};

while (true) {
    $line = $readLine();
    if ($line === null) {
        break;
    }
    $upper = strtoupper($line);
    if (str_starts_with($upper, 'EHLO ') || str_starts_with($upper, 'HELO ')) {
        if ($tls) {
            $ehloTls();
        } else {
            $ehloPlain();
        }
        continue;
    }
    if ($upper === 'STARTTLS') {
        if ($mode === 'starttls-reject') {
            $write('454 4.7.0 STARTTLS not available');
            continue;
        }
        $write('220 2.0.0 Ready to start TLS');
        if (!$enableTls()) {
            break;
        }
        $tls = true;
        continue;
    }
    if ($upper === 'AUTH LOGIN') {
        if (!$tls && $mode !== 'no-starttls') {
            $write('530 5.7.0 Must issue a STARTTLS command first');
            continue;
        }
        $write('334 VXNlcm5hbWU6');
        if ($readLine() === null) {
            break;
        }
        $write('334 UGFzc3dvcmQ6');
        if ($readLine() === null) {
            break;
        }
        if ($mode === 'auth-reject') {
            $write('535 5.7.8 Authentication failed');
            continue;
        }
        $write('235 2.7.0 Authentication successful');
        continue;
    }
    if (str_starts_with($upper, 'MAIL FROM:')) {
        $write('250 2.1.0 OK');
        continue;
    }
    if (str_starts_with($upper, 'RCPT TO:')) {
        $write('250 2.1.5 OK');
        continue;
    }
    if ($upper === 'DATA') {
        $write('354 End data with <CR><LF>.<CR><LF>');
        $payload = '';
        while (true) {
            $chunk = fgets($client, 8192);
            if ($chunk === false) {
                break;
            }
            if ($chunk === ".\r\n" || $chunk === ".\n") {
                break;
            }
            $payload .= $chunk;
        }
        if ($dataFile !== '') {
            file_put_contents($dataFile, $payload);
        }
        $write('250 2.0.0 Message accepted');
        continue;
    }
    if ($upper === 'QUIT') {
        $write('221 2.0.0 Bye');
        break;
    }
    $write('500 5.5.1 Command unrecognized');
}

if (is_resource($client)) {
    fclose($client);
}
fclose($server);
