package com.acugis.report.models;

import com.fasterxml.jackson.annotation.JsonIgnoreProperties;

import java.util.ArrayList;
import java.util.List;

@JsonIgnoreProperties(ignoreUnknown = true)
public class ReportListResponse {

    private List<ReportSummary> items = new ArrayList<>();
    private int count;

    public List<ReportSummary> getItems() {
        return items;
    }

    public void setItems(List<ReportSummary> items) {
        this.items = items == null ? new ArrayList<>() : items;
    }

    public int getCount() {
        return count;
    }

    public void setCount(int count) {
        this.count = count;
    }
}
