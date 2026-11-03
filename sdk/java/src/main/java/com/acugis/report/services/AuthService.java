package com.acugis.report.services;

import com.acugis.report.internal.HttpTransport;
import com.acugis.report.internal.QueryParams;
import com.acugis.report.models.CreateTokenResponse;
import com.acugis.report.models.SuccessResponse;
import com.acugis.report.models.TokenListResponse;
import com.acugis.report.models.TokenMetadata;
import com.acugis.report.models.User;
import com.fasterxml.jackson.databind.JsonNode;

import java.util.HashMap;
import java.util.List;
import java.util.Map;
import java.util.concurrent.CompletableFuture;

/** Authentication and personal access tokens ({@code /api/v1/auth/*}). */
public final class AuthService {

    private final HttpTransport transport;

    public AuthService(HttpTransport transport) {
        this.transport = transport;
    }

    public SuccessResponse login(String username, String password) {
        Map<String, String> body = Map.of("username", username, "password", password);
        return transport.postJson("/api/v1/auth/login", body, SuccessResponse.class);
    }

    public SuccessResponse logout() {
        return transport.postJson("/api/v1/auth/logout", Map.of(), SuccessResponse.class);
    }

    public User me() {
        return transport.getJson("/api/v1/auth/me", User.class);
    }

    public CompletableFuture<User> meAsync() {
        return transport
                .requestAsync("GET", "/api/v1/auth/me", null, null, null, true)
                .thenApply(resp -> transport.decodeJson(resp, User.class));
    }

    public List<TokenMetadata> listTokens() {
        TokenListResponse resp = transport.getJson("/api/v1/auth/tokens", TokenListResponse.class);
        return resp == null || resp.getTokens() == null ? List.of() : resp.getTokens();
    }

    public CreateTokenResponse createToken(String name) {
        return createToken(name, null, "never", null);
    }

    public CreateTokenResponse createToken(
            String name, String description, String expiresIn, String expiresAt) {
        Map<String, Object> body = new HashMap<>();
        body.put("name", name);
        body.put("expires_in", expiresIn == null ? "never" : expiresIn);
        if (description != null) {
            body.put("description", description);
        }
        if (expiresAt != null) {
            body.put("expires_at", expiresAt);
        }
        return transport.postJson("/api/v1/auth/tokens", body, CreateTokenResponse.class);
    }

    public TokenMetadata updateToken(long id, Map<String, Object> fields) {
        return transport.putJson("/api/v1/auth/tokens/" + id, fields, TokenMetadata.class);
    }

    public void deleteToken(long id) {
        deleteToken(id, false);
    }

    public void deleteToken(long id, boolean purge) {
        QueryParams q = purge ? new QueryParams().add("purge", "1") : null;
        transport.request("DELETE", "/api/v1/auth/tokens/" + id, q, null, null, true);
    }

    public CreateTokenResponse rotateToken(long id) {
        return transport.postJson("/api/v1/auth/tokens/" + id + "/rotate", Map.of(), CreateTokenResponse.class);
    }

    /**
     * Authenticate with password, then mint a PAT for subsequent Bearer auth.
     */
    public CreateTokenResponse loginAndCreateToken(String username, String password, String tokenName) {
        login(username, password);
        return createToken(tokenName, "Created by AcuGIS Report Java SDK", "never", null);
    }

    public JsonNode rawMe() {
        return transport.getJsonNode("/api/v1/auth/me");
    }
}
