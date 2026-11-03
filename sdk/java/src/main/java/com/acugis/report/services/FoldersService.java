package com.acugis.report.services;

import com.acugis.report.internal.HttpTransport;
import com.acugis.report.internal.QueryParams;
import com.acugis.report.models.BrowseResult;
import com.acugis.report.models.ContentItem;
import com.acugis.report.models.Folder;
import com.acugis.report.models.FolderTreeNode;
import com.acugis.report.models.SuccessResponse;
import com.fasterxml.jackson.core.type.TypeReference;
import com.fasterxml.jackson.databind.JsonNode;

import java.util.HashMap;
import java.util.List;
import java.util.Map;

/**
 * Folders, browse/search/tree, permissions, favorites, and content move
 * ({@code /api/v1/folders/*}, {@code /api/v1/favorites}, {@code /api/v1/content/move}).
 */
public final class FoldersService {

    private final HttpTransport transport;

    public FoldersService(HttpTransport transport) {
        this.transport = transport;
    }

    public List<FolderTreeNode> tree() {
        return transport.getJson("/api/v1/folders/tree", new TypeReference<List<FolderTreeNode>>() {});
    }

    public BrowseResult browse() {
        return browse(0L);
    }

    public BrowseResult browse(long folderId) {
        return transport.getJson(
                "/api/v1/folders/browse",
                new QueryParams().add("folder_id", folderId),
                BrowseResult.class);
    }

    public JsonNode search(String q) {
        return transport.getJsonNode("/api/v1/folders/search", new QueryParams().add("q", q));
    }

    public Folder create(String name, long parentId) {
        return create(name, parentId, null, null, null, null);
    }

    public Folder create(
            String name,
            long parentId,
            String description,
            String icon,
            Integer sortOrder,
            Boolean aclInherit) {
        Map<String, Object> body = new HashMap<>();
        body.put("name", name);
        body.put("parent_id", parentId);
        if (description != null) {
            body.put("description", description);
        }
        if (icon != null) {
            body.put("icon", icon);
        }
        if (sortOrder != null) {
            body.put("sort_order", sortOrder);
        }
        if (aclInherit != null) {
            body.put("acl_inherit", aclInherit);
        }
        return transport.postJson("/api/v1/folders", body, Folder.class);
    }

    public Folder get(long folderId) {
        return transport.getJson("/api/v1/folders/" + folderId, Folder.class);
    }

    public Folder update(long folderId, Map<String, Object> fields) {
        return transport.putJson("/api/v1/folders/" + folderId, fields, Folder.class);
    }

    public SuccessResponse delete(long folderId) {
        return delete(folderId, false, true);
    }

    public SuccessResponse delete(long folderId, boolean recursive, boolean confirm) {
        QueryParams q = new QueryParams()
                .add("recursive", recursive ? "1" : "0")
                .add("confirm", confirm ? "1" : "0");
        return transport.deleteJson("/api/v1/folders/" + folderId, q, SuccessResponse.class);
    }

    public Folder duplicate(long folderId) {
        return transport.postJson("/api/v1/folders/" + folderId + "/duplicate", Map.of(), Folder.class);
    }

    public Folder move(long folderId, long parentId) {
        return move(folderId, parentId, null);
    }

    public Folder move(long folderId, long parentId, Integer sortOrder) {
        Map<String, Object> body = new HashMap<>();
        body.put("parent_id", parentId);
        if (sortOrder != null) {
            body.put("sort_order", sortOrder);
        }
        return transport.postJson("/api/v1/folders/" + folderId + "/move", body, Folder.class);
    }

    public JsonNode permissions(long folderId) {
        return transport.getJsonNode("/api/v1/folders/" + folderId + "/permissions");
    }

    public SuccessResponse setPermissions(long folderId, List<String> groupNames, String permission) {
        Map<String, Object> body = new HashMap<>();
        body.put("group_names", groupNames);
        if (permission != null) {
            body.put("permission", permission);
        }
        return transport.putJson(
                "/api/v1/folders/" + folderId + "/permissions", body, SuccessResponse.class);
    }

    /** Place or move content into a folder ({@code POST /api/v1/content/move}). */
    public ContentItem moveContent(
            long folderId, String contentType, String contentKey, String displayName, Integer sortOrder, Boolean aclInherit) {
        Map<String, Object> body = new HashMap<>();
        body.put("folder_id", folderId);
        body.put("content_type", contentType);
        body.put("content_key", contentKey);
        if (displayName != null) {
            body.put("display_name", displayName);
        }
        if (sortOrder != null) {
            body.put("sort_order", sortOrder);
        }
        if (aclInherit != null) {
            body.put("acl_inherit", aclInherit);
        }
        return transport.postJson("/api/v1/content/move", body, ContentItem.class);
    }

    public ContentItem moveContent(long folderId, String contentType, String contentKey) {
        return moveContent(folderId, contentType, contentKey, null, null, null);
    }

    public JsonNode listFavorites() {
        return transport.getJsonNode("/api/v1/favorites");
    }

    public JsonNode setFavorite(String contentType, String contentKey, boolean favorite) {
        Map<String, Object> body = new HashMap<>();
        body.put("content_type", contentType);
        body.put("content_key", contentKey);
        body.put("favorite", favorite);
        return transport.postJson("/api/v1/favorites", body);
    }
}
