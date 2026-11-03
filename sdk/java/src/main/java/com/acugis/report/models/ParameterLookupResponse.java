package com.acugis.report.models;

import com.fasterxml.jackson.annotation.JsonIgnoreProperties;

import java.util.ArrayList;
import java.util.List;

@JsonIgnoreProperties(ignoreUnknown = true)
public class ParameterLookupResponse {

    private List<ParameterOption> options = new ArrayList<>();

    public List<ParameterOption> getOptions() {
        return options;
    }

    public void setOptions(List<ParameterOption> options) {
        this.options = options == null ? new ArrayList<>() : options;
    }
}
