package com.acugis.report.errors;

/** 404 — resource not found. */
public final class NotFoundException extends ReportServerException {

    public NotFoundException(
            String message, Integer statusCode, String responseBody, String errorCode, String hint) {
        super(message, statusCode, responseBody, errorCode, hint, null);
    }
}
