package com.acugis.report.errors;

/**
 * Base exception for AcuGIS Report Server client errors.
 */
public class ReportServerException extends RuntimeException {

    private final Integer statusCode;
    private final String responseBody;
    private final String errorCode;
    private final String hint;

    public ReportServerException(String message) {
        this(message, null, null, null, null, null);
    }

    public ReportServerException(String message, Throwable cause) {
        this(message, null, null, null, null, cause);
    }

    public ReportServerException(
            String message,
            Integer statusCode,
            String responseBody,
            String errorCode,
            String hint,
            Throwable cause) {
        super(message, cause);
        this.statusCode = statusCode;
        this.responseBody = responseBody;
        this.errorCode = errorCode;
        this.hint = hint;
    }

    public Integer getStatusCode() {
        return statusCode;
    }

    public String getResponseBody() {
        return responseBody;
    }

    public String getErrorCode() {
        return errorCode;
    }

    public String getHint() {
        return hint;
    }
}
