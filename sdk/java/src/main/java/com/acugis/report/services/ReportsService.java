package com.acugis.report.services;

import com.acugis.report.internal.HttpTransport;
import com.acugis.report.internal.MultipartBody;
import com.acugis.report.internal.QueryParams;
import com.acugis.report.internal.RawResponse;
import com.acugis.report.models.BinaryResult;
import com.acugis.report.models.ParameterLookupResponse;
import com.acugis.report.models.ReportCreateResponse;
import com.acugis.report.models.ReportFormat;
import com.acugis.report.models.ReportListResponse;
import com.acugis.report.models.ReportMutationResponse;
import com.fasterxml.jackson.databind.JsonNode;

import java.io.IOException;
import java.io.OutputStream;
import java.nio.file.Files;
import java.nio.file.Path;
import java.util.Collection;
import java.util.HashMap;
import java.util.List;
import java.util.Map;
import java.util.concurrent.CompletableFuture;

/** Reports listing, publish/update, config, and execution ({@code /api/v1/reports}, {@code /reports/execute}). */
public final class ReportsService {

    private final HttpTransport transport;

    public ReportsService(HttpTransport transport) {
        this.transport = transport;
    }

    public ReportListResponse list() {
        return transport.getJson("/api/v1/reports", ReportListResponse.class);
    }

    public CompletableFuture<ReportListResponse> listAsync() {
        return transport
                .requestAsync("GET", "/api/v1/reports", null, null, null, true)
                .thenApply(resp -> transport.decodeJson(resp, ReportListResponse.class));
    }

    public JsonNode get(String key) {
        return transport.getJsonNode("/api/v1/reports/" + encodeKey(key));
    }

    public ReportCreateResponse publish(
            Path jrxmlFile,
            String dataSource,
            String reportName,
            String reportDescription,
            String reportType,
            Path jasperFile,
            Path thumbnailFile,
            List<String> accessGroups,
            Map<String, String> extraFields) throws IOException {
        MultipartBody body = new MultipartBody();
        body.addFile("jrxml_file", jrxmlFile, "application/xml");
        body.addField("data_source", dataSource);
        if (reportName != null) {
            body.addField("report_name", reportName);
        }
        if (reportDescription != null) {
            body.addField("report_description", reportDescription);
        }
        body.addField("report_type", reportType == null ? "jasper" : reportType);
        if (jasperFile != null) {
            body.addFile("jasper_file", jasperFile, "application/octet-stream");
        }
        if (thumbnailFile != null) {
            body.addFile("thumbnail_file", thumbnailFile, "application/octet-stream");
        }
        if (accessGroups != null) {
            for (String group : accessGroups) {
                body.addField("access_groups", group);
            }
        }
        if (extraFields != null) {
            extraFields.forEach(body::addField);
        }
        return transport.postMultipart("/api/v1/reports", body, ReportCreateResponse.class);
    }

    public ReportCreateResponse publish(Path jrxmlFile, String dataSource) throws IOException {
        return publish(jrxmlFile, dataSource, null, null, "jasper", null, null, null, null);
    }

    public ReportMutationResponse update(
            String key,
            Path jrxmlFile,
            String dataSource,
            String reportName,
            String reportDescription,
            String reportType,
            Path jasperFile,
            Path thumbnailFile,
            List<String> accessGroups) throws IOException {
        MultipartBody body = new MultipartBody();
        if (jrxmlFile != null) {
            body.addFile("jrxml_file", jrxmlFile, "application/xml");
        }
        if (dataSource != null) {
            body.addField("data_source", dataSource);
        }
        if (reportName != null) {
            body.addField("report_name", reportName);
        }
        if (reportDescription != null) {
            body.addField("report_description", reportDescription);
        }
        if (reportType != null) {
            body.addField("report_type", reportType);
        }
        if (jasperFile != null) {
            body.addFile("jasper_file", jasperFile, "application/octet-stream");
        }
        if (thumbnailFile != null) {
            body.addFile("thumbnail_file", thumbnailFile, "application/octet-stream");
        }
        if (accessGroups != null) {
            for (String group : accessGroups) {
                body.addField("access_groups", group);
            }
        }
        return transport.putMultipart("/api/v1/reports/" + encodeKey(key), body, ReportMutationResponse.class);
    }

    public ReportMutationResponse delete(String key) {
        return transport.deleteJson("/api/v1/reports/" + encodeKey(key), null, ReportMutationResponse.class);
    }

    public JsonNode config(String key) {
        return transport.getJsonNode("/api/v1/reports/" + encodeKey(key) + "/config");
    }

    public byte[] thumbnail(String key) {
        return transport.getBytes("/api/v1/reports/" + encodeKey(key) + "/thumbnail", null);
    }

    public ParameterLookupResponse lookupParameter(String key, String name, Map<String, ?> parentParams) {
        String path = "/api/v1/reports/" + encodeKey(key) + "/parameters/" + encodeKey(name) + "/lookup";
        if (parentParams == null || parentParams.isEmpty()) {
            return transport.getJson(path, ParameterLookupResponse.class);
        }
        Map<String, Object> body = new HashMap<>();
        body.put("params", parentParams);
        return transport.postJson(path, body, ParameterLookupResponse.class);
    }

    /**
     * Execute a report via {@code GET /reports/execute}.
     * Multi-value parameters (Collection/array) are repeated in the query string.
     */
    public BinaryResult run(String key, Map<String, ?> params, String format) {
        return run(key, params, format, "GET");
    }

    public BinaryResult run(String key, Map<String, ?> params, String format, String method) {
        ReportFormat fmt = ReportFormat.parse(format);
        QueryParams query = new QueryParams()
                .add("_repName", key)
                .add("_repFormat", fmt.wireValue());
        if (params != null) {
            for (Map.Entry<String, ?> e : params.entrySet()) {
                Object v = e.getValue();
                if (v instanceof Collection<?> || (v != null && v.getClass().isArray())) {
                    query.add(e.getKey(), v);
                } else if (v != null) {
                    query.add(e.getKey(), v);
                }
            }
        }
        String m = method == null ? "GET" : method.toUpperCase();
        RawResponse resp = transport.request(m, "/reports/execute", query, null, null, true);
        return new BinaryResult(resp.body(), resp.contentType().orElse(null));
    }

    public CompletableFuture<BinaryResult> runAsync(String key, Map<String, ?> params, String format) {
        ReportFormat fmt = ReportFormat.parse(format);
        QueryParams query = new QueryParams()
                .add("_repName", key)
                .add("_repFormat", fmt.wireValue())
                .addAll(params);
        return transport
                .requestAsync("GET", "/reports/execute", query, null, null, true)
                .thenApply(resp -> new BinaryResult(resp.body(), resp.contentType().orElse(null)));
    }

    /** Run report as PDF (default) and write bytes to {@code destination}. */
    public Path runToFile(String key, Map<String, ?> params, Path destination) throws IOException {
        return runToFile(key, params, destination, "pdf");
    }

    public Path runToFile(String key, Map<String, ?> params, Path destination, String format)
            throws IOException {
        BinaryResult result = run(key, params, format);
        if (destination.getParent() != null) {
            Files.createDirectories(destination.getParent());
        }
        Files.write(destination, result.getContent());
        return destination;
    }

    public void runToStream(String key, Map<String, ?> params, String format, OutputStream out)
            throws IOException {
        BinaryResult result = run(key, params, format);
        out.write(result.getContent());
    }

    private static String encodeKey(String key) {
        return java.net.URLEncoder.encode(key, java.nio.charset.StandardCharsets.UTF_8)
                .replace("+", "%20");
    }
}
