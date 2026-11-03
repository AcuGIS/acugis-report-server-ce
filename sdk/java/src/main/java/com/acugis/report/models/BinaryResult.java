package com.acugis.report.models;

import java.util.Optional;

/** Binary response with optional Content-Type (e.g. report render, resource download). */
public final class BinaryResult {

    private final byte[] content;
    private final String contentType;

    public BinaryResult(byte[] content, String contentType) {
        this.content = content == null ? new byte[0] : content;
        this.contentType = contentType;
    }

    public byte[] getContent() {
        return content;
    }

    public Optional<String> getContentType() {
        return Optional.ofNullable(contentType);
    }

    public int size() {
        return content.length;
    }
}
