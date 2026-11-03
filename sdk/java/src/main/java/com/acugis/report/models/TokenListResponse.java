package com.acugis.report.models;

import com.fasterxml.jackson.annotation.JsonIgnoreProperties;

import java.util.ArrayList;
import java.util.List;

@JsonIgnoreProperties(ignoreUnknown = true)
public class TokenListResponse {

    private List<TokenMetadata> tokens = new ArrayList<>();

    public List<TokenMetadata> getTokens() {
        return tokens;
    }

    public void setTokens(List<TokenMetadata> tokens) {
        this.tokens = tokens == null ? new ArrayList<>() : tokens;
    }
}
