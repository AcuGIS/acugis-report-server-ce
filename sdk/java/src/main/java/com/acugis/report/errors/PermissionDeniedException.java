package com.acugis.report.errors;

/** 403 — permission denied. */
public final class PermissionDeniedException extends ReportServerException {

    public PermissionDeniedException(
            String message, Integer statusCode, String responseBody, String errorCode, String hint) {
        super(message, statusCode, responseBody, errorCode, hint, null);
    }
}
