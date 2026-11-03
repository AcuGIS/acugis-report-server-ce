package com.acugis.report.services;

import com.acugis.report.errors.NotFoundException;
import com.acugis.report.errors.NotImplementedByServerException;
import com.acugis.report.errors.ReportServerException;
import com.acugis.report.internal.HttpTransport;
import com.acugis.report.internal.QueryParams;
import com.acugis.report.models.DatasourceName;
import com.acugis.report.models.JasperContextResponse;
import com.acugis.report.models.SuccessResponse;
import com.fasterxml.jackson.core.type.TypeReference;
import com.fasterxml.jackson.databind.JsonNode;

import java.util.List;
import java.util.Map;

/**
 * Datasources ({@code /api/v1/datasources*}).
 * List returns names only; create is form-urlencoded upsert; no GET-by-id / PUT yet.
 */
public final class DatasourcesService {

    private final HttpTransport transport;

    public DatasourcesService(HttpTransport transport) {
        this.transport = transport;
    }

    public List<DatasourceName> list() {
        return transport.getJson("/api/v1/datasources", new TypeReference<List<DatasourceName>>() {});
    }

    /**
     * Create or upsert a datasource. Body is {@code application/x-www-form-urlencoded}.
     */
    public SuccessResponse create(Map<String, ?> fields) {
        QueryParams form = new QueryParams().addAll(fields);
        return transport.postForm("/api/v1/datasources", form, SuccessResponse.class);
    }

    public SuccessResponse delete(long id) {
        return transport.deleteJson("/api/v1/datasources/" + id, null, SuccessResponse.class);
    }

    public JasperContextResponse generateContext() {
        return transport.postJson("/api/v1/datasources/context", Map.of(), JasperContextResponse.class);
    }

    /** Reserved: activates when {@code GET /api/v1/datasources/{id}} exists. */
    public JsonNode get(long id) {
        try {
            return transport.getJsonNode("/api/v1/datasources/" + id);
        } catch (NotFoundException e) {
            throw new NotImplementedByServerException("datasources show");
        } catch (ReportServerException e) {
            if (e.getStatusCode() != null && e.getStatusCode() == 405) {
                throw new NotImplementedByServerException("datasources show");
            }
            throw e;
        }
    }

    /** Reserved: activates when {@code PUT /api/v1/datasources/{id}} exists. */
    public JsonNode update(long id, Map<String, Object> fields) {
        try {
            return transport.putJson("/api/v1/datasources/" + id, fields);
        } catch (NotFoundException e) {
            throw new NotImplementedByServerException("datasources update");
        } catch (ReportServerException e) {
            if (e.getStatusCode() != null && e.getStatusCode() == 405) {
                throw new NotImplementedByServerException("datasources update");
            }
            throw e;
        }
    }
}
