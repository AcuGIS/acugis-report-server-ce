package com.acugis.report.errors;

/**
 * Thrown for reserved service surfaces (users, groups, schedules, jobs, recipients)
 * that are not yet implemented by the server REST API.
 */
public final class NotImplementedByServerException extends ReportServerException {

    private final String feature;

    public NotImplementedByServerException(String feature) {
        super(feature + " is not implemented by the AcuGIS Report Server REST API yet.");
        this.feature = feature;
    }

    public String getFeature() {
        return feature;
    }
}
