package com.acugis.report.services;

import com.acugis.report.internal.HttpTransport;
import com.acugis.report.internal.QueryParams;
import com.acugis.report.models.BinaryResult;
import com.fasterxml.jackson.databind.JsonNode;

import java.util.HashMap;
import java.util.Map;

/** Map report features, extent, columns, and map-aware report execution. */
public final class MapReportsService {

    private final HttpTransport transport;
    private final ReportsService reports;

    public MapReportsService(HttpTransport transport, ReportsService reports) {
        this.transport = transport;
        this.reports = reports;
    }

    public JsonNode features(String key, Map<String, ?> params) {
        QueryParams q = new QueryParams().addAll(params);
        return transport.getJsonNode("/api/v1/map/" + encode(key) + "/features", q.isEmpty() ? null : q);
    }

    public JsonNode features(String key) {
        return features(key, null);
    }

    public JsonNode extent(
            String key,
            Integer srid,
            String geometry,
            Double xmin,
            Double ymin,
            Double xmax,
            Double ymax) {
        QueryParams q = new QueryParams();
        if (srid != null) {
            q.add("srid", srid);
        }
        if (geometry != null) {
            q.add("geometry", geometry);
        }
        if (xmin != null) {
            q.add("xmin", xmin);
        }
        if (ymin != null) {
            q.add("ymin", ymin);
        }
        if (xmax != null) {
            q.add("xmax", xmax);
        }
        if (ymax != null) {
            q.add("ymax", ymax);
        }
        return transport.getJsonNode("/api/v1/map/" + encode(key) + "/extent", q.isEmpty() ? null : q);
    }

    public JsonNode columns(String key) {
        return transport.getJsonNode("/api/v1/map/" + encode(key) + "/columns");
    }

    /**
     * Run a map report, injecting {@code p_geometry} / {@code p_srid} into report parameters.
     */
    public BinaryResult run(
            String key, String geometry, Integer srid, String format, Map<String, ?> params) {
        Map<String, Object> runParams = new HashMap<>();
        if (params != null) {
            runParams.putAll(params);
        }
        if (geometry != null) {
            runParams.put("p_geometry", geometry);
        }
        if (srid != null) {
            runParams.put("p_srid", srid);
        }
        return reports.run(key, runParams, format == null ? "pdf" : format);
    }

    private static String encode(String key) {
        return java.net.URLEncoder.encode(key, java.nio.charset.StandardCharsets.UTF_8)
                .replace("+", "%20");
    }
}
