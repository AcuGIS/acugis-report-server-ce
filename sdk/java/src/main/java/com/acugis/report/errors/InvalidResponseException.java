package com.acugis.report.errors;

/** Response could not be parsed as expected. */
public final class InvalidResponseException extends ReportServerException {

    public InvalidResponseException(String message, Integer statusCode, String responseBody, Throwable cause) {
        super(message, statusCode, responseBody, null, null, cause);
    }

    public InvalidResponseException(String message) {
        super(message);
    }
}
