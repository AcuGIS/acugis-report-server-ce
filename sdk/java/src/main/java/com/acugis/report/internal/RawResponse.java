package com.acugis.report.internal;

import java.util.Optional;

/** Low-level HTTP response. */
public final class RawResponse {

    private final int statusCode;
    private final byte[] body;
    private final String contentType;
    private final java.net.http.HttpHeaders headers;

    public RawResponse(int statusCode, byte[] body, String contentType, java.net.http.HttpHeaders headers) {
        this.statusCode = statusCode;
        this.body = body == null ? new byte[0] : body;
        this.contentType = contentType;
        this.headers = headers;
    }

    public int statusCode() {
        return statusCode;
    }

    public byte[] body() {
        return body;
    }

    public Optional<String> contentType() {
        return Optional.ofNullable(contentType);
    }

    public java.net.http.HttpHeaders headers() {
        return headers;
    }

    public String bodyAsString() {
        return new String(body, java.nio.charset.StandardCharsets.UTF_8);
    }
}
