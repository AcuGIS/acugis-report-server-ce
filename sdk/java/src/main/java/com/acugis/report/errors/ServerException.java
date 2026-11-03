package com.acugis.report.errors;

/** 5xx — server or dependency failure. */
public final class ServerException extends ReportServerException {

    public ServerException(
            String message, Integer statusCode, String responseBody, String errorCode, String hint) {
        super(message, statusCode, responseBody, errorCode, hint, null);
    }

    public ServerException(String message, Integer statusCode, String responseBody, Throwable cause) {
        super(message, statusCode, responseBody, null, null, cause);
    }
}
