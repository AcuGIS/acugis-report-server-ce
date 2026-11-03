package com.acugis.report.errors;

import java.util.Collections;
import java.util.List;

/** 400 / 422 — validation or bad request. */
public final class ValidationException extends ReportServerException {

    private final List<String> errors;

    public ValidationException(
            String message,
            Integer statusCode,
            String responseBody,
            String errorCode,
            String hint,
            List<String> errors) {
        super(message, statusCode, responseBody, errorCode, hint, null);
        this.errors = errors == null ? List.of() : Collections.unmodifiableList(errors);
    }

    public ValidationException(String message) {
        this(message, null, null, null, null, List.of());
    }

    /** Field-level validation messages when present. */
    public List<String> getErrors() {
        return errors;
    }
}
