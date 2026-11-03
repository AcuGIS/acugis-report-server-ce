package com.acugis.report.errors;

/** Transport-level failure (DNS, connection reset, TLS, etc.). */
public final class NetworkException extends ReportServerException {

    public NetworkException(String message, Throwable cause) {
        super(message, null, null, null, null, cause);
    }

    public NetworkException(String message) {
        super(message);
    }
}
