package com.acugis.report.services;

import com.acugis.report.internal.HttpTransport;
import com.acugis.report.internal.QueryParams;
import com.fasterxml.jackson.databind.JsonNode;

/** Embedded map debug ({@code GET /api/v1/embedded-map/debug}). */
public final class EmbeddedMapsService {

    private final HttpTransport transport;

    public EmbeddedMapsService(HttpTransport transport) {
        this.transport = transport;
    }

    public JsonNode debug(String report) {
        return debug(report, false);
    }

    public JsonNode debug(String report, boolean compile) {
        QueryParams q = new QueryParams().add("report", report).add("format", "json");
        if (compile) {
            q.add("compile", "1");
        }
        return transport.getJsonNode("/api/v1/embedded-map/debug", q);
    }
}
