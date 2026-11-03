package com.acugis.report.models;

import com.fasterxml.jackson.annotation.JsonIgnoreProperties;
import com.fasterxml.jackson.annotation.JsonProperty;
import com.fasterxml.jackson.databind.JsonNode;

import java.util.List;

/** Authenticated user identity ({@code GET /api/v1/auth/me}). */
@JsonIgnoreProperties(ignoreUnknown = true)
public class User {

    private Long id;
    private String username;
    private String email;
    private List<String> groups;

    @JsonProperty("is_admin")
    private Boolean isAdmin;

    private JsonNode permissions;

    @JsonProperty("auth_disabled")
    private Boolean authDisabled;

    public Long getId() {
        return id;
    }

    public void setId(Long id) {
        this.id = id;
    }

    public String getUsername() {
        return username;
    }

    public void setUsername(String username) {
        this.username = username;
    }

    public String getEmail() {
        return email;
    }

    public void setEmail(String email) {
        this.email = email;
    }

    public List<String> getGroups() {
        return groups;
    }

    public void setGroups(List<String> groups) {
        this.groups = groups;
    }

    public Boolean getIsAdmin() {
        return isAdmin;
    }

    public void setIsAdmin(Boolean isAdmin) {
        this.isAdmin = isAdmin;
    }

    public JsonNode getPermissions() {
        return permissions;
    }

    public void setPermissions(JsonNode permissions) {
        this.permissions = permissions;
    }

    public Boolean getAuthDisabled() {
        return authDisabled;
    }

    public void setAuthDisabled(Boolean authDisabled) {
        this.authDisabled = authDisabled;
    }
}
