package com.acugis.report.errors;

/** 409 — conflict (e.g. folder not empty, name clash). */
public final class ConflictException extends ReportServerException {

    public ConflictException(
            String message, Integer statusCode, String responseBody, String errorCode, String hint) {
        super(message, statusCode, responseBody, errorCode, hint, null);
    }
}
