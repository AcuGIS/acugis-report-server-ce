package com.acugis.report.services;

import com.acugis.report.errors.ReportServerException;
import com.acugis.report.internal.HttpTransport;
import com.acugis.report.internal.RawResponse;
import com.acugis.report.models.HealthResponse;
import com.acugis.report.models.ReadyResponse;
import com.fasterxml.jackson.databind.JsonNode;
import com.fasterxml.jackson.databind.node.ArrayNode;
import com.fasterxml.jackson.databind.node.ObjectNode;

import java.util.Iterator;
import java.util.Map;

/** Health, readiness, diagnostics, and Jasper status. */
public final class HealthService {

    private final HttpTransport transport;

    public HealthService(HttpTransport transport) {
        this.transport = transport;
    }

    public HealthResponse health() {
        return transport.getJson("/health", HealthResponse.class);
    }

    public ReadyResponse ready() {
        RawResponse resp = transport.request("GET", "/ready", null, null, null, false);
        return transport.decodeJson(resp, ReadyResponse.class);
    }

    public JsonNode diagnostics() {
        return transport.getJsonNode("/api/v1/diagnostics");
    }

    public JsonNode jasperStatus() {
        return transport.getJsonNode("/dev/jasper-status");
    }

    /**
     * Aggregate health, readiness, diagnostics, and Jasper status into a single doctor report.
     */
    public JsonNode doctor() {
        ObjectNode result = com.acugis.report.internal.JsonSupport.mapper().createObjectNode();
        ObjectNode checks = result.putObject("checks");
        ArrayNode warnings = result.putArray("warnings");
        result.put("ok", true);

        safe(checks, warnings, result, "health", () -> transport.getJsonNode("/health"));
        safe(checks, warnings, result, "ready", () -> {
            RawResponse resp = transport.request("GET", "/ready", null, null, null, false);
            return transport.decodeJsonNode(resp);
        });
        safe(checks, warnings, result, "diagnostics", () -> transport.getJsonNode("/api/v1/diagnostics"));
        safe(checks, warnings, result, "jasper", () -> transport.getJsonNode("/dev/jasper-status"));

        JsonNode ready = checks.get("ready");
        if (ready != null && ready.has("checks") && ready.get("checks").isObject()) {
            Iterator<Map.Entry<String, JsonNode>> fields = ready.get("checks").fields();
            while (fields.hasNext()) {
                Map.Entry<String, JsonNode> entry = fields.next();
                if (!looksOk(entry.getValue())) {
                    warnings.add("readiness:" + entry.getKey());
                    result.put("ok", false);
                }
            }
        }
        return result;
    }

    private void safe(
            ObjectNode checks,
            ArrayNode warnings,
            ObjectNode result,
            String name,
            SupplierWithException supplier) {
        try {
            checks.set(name, supplier.get());
        } catch (ReportServerException e) {
            ObjectNode err = checks.putObject(name);
            err.put("error", e.getMessage());
            if (e.getStatusCode() != null) {
                err.put("status_code", e.getStatusCode());
            }
            result.put("ok", false);
            warnings.add(name + ": " + e.getMessage());
        } catch (Exception e) {
            ObjectNode err = checks.putObject(name);
            err.put("error", e.getMessage() == null ? e.toString() : e.getMessage());
            result.put("ok", false);
            warnings.add(name + ": " + e.getMessage());
        }
    }

    private static boolean looksOk(JsonNode value) {
        if (value == null || value.isNull()) {
            return false;
        }
        if (value.isBoolean()) {
            return value.asBoolean();
        }
        if (value.isTextual()) {
            String s = value.asText().toLowerCase();
            return s.equals("ok") || s.equals("ready") || s.equals("pass") || s.equals("true");
        }
        if (value.isObject()) {
            if (value.has("status")) {
                return looksOk(value.get("status"));
            }
            if (value.has("ok")) {
                return looksOk(value.get("ok"));
            }
        }
        return true;
    }

    @FunctionalInterface
    private interface SupplierWithException {
        JsonNode get() throws Exception;
    }
}
