package com.acugis.report.errors;

/** Request exceeded the configured timeout. */
public final class TimeoutException extends ReportServerException {

    public TimeoutException(String message, Throwable cause) {
        super(message, null, null, null, null, cause);
    }

    public TimeoutException(String message) {
        super(message);
    }
}
