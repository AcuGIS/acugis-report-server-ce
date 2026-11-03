<?php
declare(strict_types=1);

/**
 * Send scheduled report email with attachment (mail() or minimal SMTP).
 *
 * @param list<string> $recipients
 */
function jrs_schedule_send_mail(
    array $recipients,
    string $subject,
    string $body,
    string $attachmentPath,
    string $attachmentName,
    bool $bodyIsHtml = false
): array {
    $recipients = array_values(array_filter(array_map('trim', $recipients)));
    if ($recipients === []) {
        return ['ok' => false, 'message' => 'No recipients'];
    }
    if (!is_file($attachmentPath)) {
        return ['ok' => false, 'message' => 'Attachment missing'];
    }

    $smtpHost = trim((string) jrs_config('smtp_host', ''));
    if ($smtpHost !== '') {
        return jrs_schedule_send_smtp($recipients, $subject, $body, $attachmentPath, $attachmentName, $bodyIsHtml);
    }

    $from = trim((string) jrs_config('smtp_from', ''));
    if ($from === '') {
        $from = 'jasper-reports@localhost';
    }

    $boundary = 'jrs_' . bin2hex(random_bytes(8));
    $headers = [
        'From: ' . $from,
        'MIME-Version: 1.0',
        'Content-Type: multipart/mixed; boundary="' . $boundary . '"',
    ];

    $bodyContentType = $bodyIsHtml ? 'text/html' : 'text/plain';
    $fileData = chunk_split(base64_encode((string) file_get_contents($attachmentPath)));
    $mime = "--{$boundary}\r\n";
    $mime .= "Content-Type: {$bodyContentType}; charset=utf-8\r\n\r\n";
    $mime .= $body . "\r\n";
    $mime .= "--{$boundary}\r\n";
    $mime .= 'Content-Type: application/octet-stream; name="' . basename($attachmentName) . "\"\r\n";
    $mime .= "Content-Transfer-Encoding: base64\r\n";
    $mime .= 'Content-Disposition: attachment; filename="' . basename($attachmentName) . "\"\r\n\r\n";
    $mime .= $fileData . "\r\n";
    $mime .= "--{$boundary}--";

    $ok = true;
    $errors = [];
    foreach ($recipients as $to) {
        if (!@mail($to, $subject, $mime, implode("\r\n", $headers))) {
            $ok = false;
            $errors[] = $to;
        }
    }

    return [
        'ok' => $ok,
        'message' => $ok ? 'sent' : ('mail failed for: ' . implode(', ', $errors)),
    ];
}

/**
 * @param list<string> $recipients
 * @param array<string, mixed>|null $sslOptions test-only TLS context overrides
 */
function jrs_schedule_send_smtp(
    array $recipients,
    string $subject,
    string $body,
    string $attachmentPath,
    string $attachmentName,
    bool $bodyIsHtml = false,
    ?array $sslOptions = null
): array {
    $host = trim((string) jrs_config('smtp_host', ''));
    $port = (int) jrs_config('smtp_port', 587);
    $user = (string) jrs_config('smtp_user', '');
    $pass = (string) jrs_config('smtp_pass', '');
    $from = trim((string) jrs_config('smtp_from', ''));
    if ($from === '') {
        $from = $user !== '' ? $user : 'jasper-reports@localhost';
    }

    $errno = 0;
    $errstr = '';
    $remote = jrs_schedule_smtp_remote($host, $port);
    $ctx = stream_context_create(['ssl' => jrs_schedule_smtp_ssl_options($host, $sslOptions)]);
    $fp = @stream_socket_client($remote, $errno, $errstr, 20, STREAM_CLIENT_CONNECT, $ctx);
    if (!$fp) {
        return ['ok' => false, 'message' => 'SMTP connect failed: ' . $errstr];
    }
    stream_set_timeout($fp, 20);

    try {
        return jrs_schedule_smtp_session(
            $fp,
            $host,
            $port,
            $user,
            $pass,
            $from,
            $recipients,
            $subject,
            $body,
            $attachmentPath,
            $attachmentName,
            $bodyIsHtml
        );
    } finally {
        if (is_resource($fp)) {
            fclose($fp);
        }
    }
}

/**
 * @param resource $fp
 * @param list<string> $recipients
 * @return array{ok:bool, message:string}
 */
function jrs_schedule_smtp_session(
    $fp,
    string $host,
    int $port,
    string $user,
    string $pass,
    string $from,
    array $recipients,
    string $subject,
    string $body,
    string $attachmentPath,
    string $attachmentName,
    bool $bodyIsHtml
): array {
    $fail = static function (string $message) use ($pass): array {
        return ['ok' => false, 'message' => jrs_schedule_smtp_redact($message, $pass)];
    };

    $greeting = jrs_schedule_smtp_read($fp);
    if (jrs_schedule_smtp_code($greeting) !== 220) {
        return $fail(jrs_schedule_smtp_stage_error('SMTP greeting failed', $greeting));
    }

    $helo = jrs_schedule_smtp_helo($from, $host);
    $ehlo = jrs_schedule_smtp_cmd($fp, 'EHLO ' . $helo);
    if (jrs_schedule_smtp_code($ehlo) !== 250) {
        return $fail(jrs_schedule_smtp_stage_error('SMTP EHLO failed', $ehlo));
    }

    // Match Go sendMailContext: STARTTLS when advertised. Port 587 (submission)
    // must upgrade before AUTH LOGIN.
    $starttlsOffered = jrs_schedule_smtp_has_capability($ehlo, 'STARTTLS');
    if ($port === 587 && !$starttlsOffered) {
        return $fail('SMTP STARTTLS not supported');
    }
    if ($starttlsOffered) {
        $tlsResp = jrs_schedule_smtp_cmd($fp, 'STARTTLS');
        if (jrs_schedule_smtp_code($tlsResp) !== 220) {
            return $fail(jrs_schedule_smtp_stage_error('SMTP STARTTLS rejected', $tlsResp));
        }
        if (!jrs_schedule_smtp_enable_tls($fp)) {
            return $fail('SMTP TLS handshake failed');
        }
        $ehlo = jrs_schedule_smtp_cmd($fp, 'EHLO ' . $helo);
        if (jrs_schedule_smtp_code($ehlo) !== 250) {
            return $fail(jrs_schedule_smtp_stage_error('SMTP EHLO failed', $ehlo));
        }
    }

    if ($user !== '') {
        $auth = jrs_schedule_smtp_cmd($fp, 'AUTH LOGIN');
        if (jrs_schedule_smtp_code($auth) !== 334) {
            return $fail(jrs_schedule_smtp_stage_error('SMTP authentication rejected', $auth));
        }
        $userResp = jrs_schedule_smtp_cmd($fp, base64_encode($user));
        if (jrs_schedule_smtp_code($userResp) !== 334) {
            return $fail(jrs_schedule_smtp_stage_error('SMTP authentication rejected', $userResp));
        }
        $passResp = jrs_schedule_smtp_cmd($fp, base64_encode($pass));
        if (jrs_schedule_smtp_code($passResp) !== 235) {
            return $fail(jrs_schedule_smtp_stage_error('SMTP authentication rejected', $passResp));
        }
    }

    $fileData = chunk_split(base64_encode((string) file_get_contents($attachmentPath)));
    $boundary = 'jrs_' . bin2hex(random_bytes(8));
    $toList = implode(', ', $recipients);

    $mailFrom = jrs_schedule_smtp_cmd($fp, 'MAIL FROM:<' . $from . '>');
    if (jrs_schedule_smtp_code($mailFrom) !== 250) {
        return $fail(jrs_schedule_smtp_stage_error('SMTP MAIL FROM rejected', $mailFrom));
    }
    foreach ($recipients as $to) {
        $rcpt = jrs_schedule_smtp_cmd($fp, 'RCPT TO:<' . $to . '>');
        $rcptCode = jrs_schedule_smtp_code($rcpt);
        if ($rcptCode !== 250 && $rcptCode !== 251) {
            return $fail(jrs_schedule_smtp_stage_error('SMTP RCPT TO rejected', $rcpt));
        }
    }
    $data = jrs_schedule_smtp_cmd($fp, 'DATA');
    if (jrs_schedule_smtp_code($data) !== 354) {
        return $fail(jrs_schedule_smtp_stage_error('SMTP DATA rejected', $data));
    }

    $message = "From: {$from}\r\n";
    $message .= "To: {$toList}\r\n";
    $message .= 'Subject: ' . $subject . "\r\n";
    $message .= "MIME-Version: 1.0\r\n";
    $message .= 'Content-Type: multipart/mixed; boundary="' . $boundary . "\"\r\n\r\n";
    $bodyContentType = $bodyIsHtml ? 'text/html' : 'text/plain';
    $message .= "--{$boundary}\r\nContent-Type: {$bodyContentType}; charset=utf-8\r\n\r\n{$body}\r\n";
    $message .= "--{$boundary}\r\n";
    $message .= 'Content-Type: application/octet-stream; name="' . basename($attachmentName) . "\"\r\n";
    $message .= "Content-Transfer-Encoding: base64\r\n";
    $message .= 'Content-Disposition: attachment; filename="' . basename($attachmentName) . "\"\r\n\r\n";
    $message .= $fileData . "\r\n--{$boundary}--\r\n";
    $message .= ".\r\n";

    fwrite($fp, $message);
    $resp = jrs_schedule_smtp_read($fp);
    if (jrs_schedule_smtp_code($resp) !== 250) {
        return $fail(jrs_schedule_smtp_stage_error('SMTP message rejected', $resp));
    }
    jrs_schedule_smtp_cmd($fp, 'QUIT');

    return ['ok' => true, 'message' => 'sent'];
}

function jrs_schedule_smtp_remote(string $host, int $port): string
{
    if (str_contains($host, ':') && !str_starts_with($host, '[')) {
        return 'tcp://[' . $host . ']:' . $port;
    }
    return 'tcp://' . $host . ':' . $port;
}

/**
 * TLS 1.2+ with hostname verification, matching Go tls.Config{ServerName, MinVersion: TLS 1.2}.
 *
 * @param array<string, mixed>|null $overrides
 * @return array<string, mixed>
 */
function jrs_schedule_smtp_ssl_options(string $host, ?array $overrides = null): array
{
    $method = STREAM_CRYPTO_METHOD_TLSv1_2_CLIENT;
    if (defined('STREAM_CRYPTO_METHOD_TLSv1_3_CLIENT')) {
        $method |= STREAM_CRYPTO_METHOD_TLSv1_3_CLIENT;
    }
    $opts = [
        'peer_name' => $host,
        'verify_peer' => true,
        'verify_peer_name' => true,
        'allow_self_signed' => false,
        'SNI_enabled' => true,
        'disable_compression' => true,
        'crypto_method' => $method,
    ];
    if ($overrides !== null) {
        foreach ($overrides as $key => $value) {
            $opts[$key] = $value;
        }
    }
    return $opts;
}

/**
 * EHLO name: never prefer localhost (Go sendMailContext / heloNameFromAddress).
 */
function jrs_schedule_smtp_helo(string $from, string $host): string
{
    $configured = trim((string) jrs_config('smtp_helo', getenv('JRS_SMTP_HELO') ?: ''));
    if ($configured !== '' && strcasecmp($configured, 'localhost') !== 0) {
        return $configured;
    }
    $at = strrpos($from, '@');
    if ($at !== false) {
        $domain = trim(substr($from, $at + 1), "[]> \t");
        if ($domain !== '' && strcasecmp($domain, 'localhost') !== 0) {
            return $domain;
        }
    }
    $host = trim($host);
    if ($host !== '' && strcasecmp($host, 'localhost') !== 0) {
        return $host;
    }
    return 'localhost';
}

/**
 * @param resource $fp
 */
function jrs_schedule_smtp_read($fp): string
{
    $data = '';
    while (($line = fgets($fp, 515)) !== false) {
        $data .= $line;
        if (isset($line[3]) && $line[3] === ' ') {
            break;
        }
    }
    return $data;
}

/**
 * @param resource $fp
 */
function jrs_schedule_smtp_cmd($fp, string $cmd): string
{
    fwrite($fp, $cmd . "\r\n");
    return jrs_schedule_smtp_read($fp);
}

function jrs_schedule_smtp_code(string $resp): int
{
    $resp = ltrim($resp);
    if ($resp === '' || !ctype_digit(substr($resp, 0, 3))) {
        return 0;
    }
    return (int) substr($resp, 0, 3);
}

function jrs_schedule_smtp_has_capability(string $ehloResp, string $name): bool
{
    foreach (preg_split("/\r\n|\n|\r/", $ehloResp) ?: [] as $line) {
        if (preg_match('/^250[\s-]' . preg_quote($name, '/') . '(?:\s|$)/i', $line) === 1) {
            return true;
        }
    }
    return false;
}

/**
 * @param resource $fp
 */
function jrs_schedule_smtp_enable_tls($fp): bool
{
    $method = STREAM_CRYPTO_METHOD_TLSv1_2_CLIENT;
    if (defined('STREAM_CRYPTO_METHOD_TLSv1_3_CLIENT')) {
        $method |= STREAM_CRYPTO_METHOD_TLSv1_3_CLIENT;
    }
    $crypto = @stream_socket_enable_crypto($fp, true, $method);
    $deadline = microtime(true) + 20;
    while ($crypto === 0 && microtime(true) < $deadline) {
        $crypto = @stream_socket_enable_crypto($fp, true, $method);
    }
    return $crypto === true;
}

function jrs_schedule_smtp_stage_error(string $prefix, string $resp): string
{
    $detail = trim($resp);
    if ($detail === '') {
        return $prefix . ': connection closed';
    }
    return $prefix . ': ' . $detail;
}

function jrs_schedule_smtp_redact(string $message, string $pass): string
{
    if ($pass === '') {
        return $message;
    }
    foreach ([$pass, base64_encode($pass)] as $needle) {
        if ($needle !== '' && str_contains($message, $needle)) {
            $message = str_replace($needle, '***', $message);
        }
    }
    return $message;
}

function jrs_schedule_parse_recipients(string $emailField): array
{
    $parts = preg_split('/[,;\s]+/', $emailField) ?: [];
    $out = [];
    foreach ($parts as $p) {
        $p = trim($p);
        if ($p !== '' && filter_var($p, FILTER_VALIDATE_EMAIL)) {
            $out[] = $p;
        }
    }
    return array_values(array_unique($out));
}
