package com.acugis.report.errors;

/** 401 — authentication required or invalid credentials. */
public final class AuthenticationException extends ReportServerException {

    public AuthenticationException(
            String message, Integer statusCode, String responseBody, String errorCode, String hint) {
        super(message, statusCode, responseBody, errorCode, hint, null);
    }
}
