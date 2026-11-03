package com.acugis.report.models;

import com.fasterxml.jackson.annotation.JsonIgnoreProperties;
import com.fasterxml.jackson.databind.JsonNode;

@JsonIgnoreProperties(ignoreUnknown = true)
public class ReadyResponse {

    private String status;
    private String service;
    private JsonNode checks;

    public String getStatus() {
        return status;
    }

    public void setStatus(String status) {
        this.status = status;
    }

    public String getService() {
        return service;
    }

    public void setService(String service) {
        this.service = service;
    }

    public JsonNode getChecks() {
        return checks;
    }

    public void setChecks(JsonNode checks) {
        this.checks = checks;
    }
}
