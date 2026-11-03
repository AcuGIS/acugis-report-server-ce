package com.acugis.report.services;

import com.acugis.report.internal.HttpTransport;
import com.acugis.report.internal.MultipartBody;
import com.acugis.report.internal.QueryParams;
import com.acugis.report.internal.RawResponse;
import com.acugis.report.models.BinaryResult;
import com.acugis.report.models.Resource;
import com.acugis.report.models.ResourceListResponse;
import com.acugis.report.models.SuccessResponse;
import com.fasterxml.jackson.databind.JsonNode;

import java.io.IOException;
import java.nio.file.Path;
import java.util.HashMap;
import java.util.Map;

/**
 * Content resources, images, download-to-Path, and resource sync
 * ({@code /api/v1/resources/*}, {@code /api/v1/resource-sync}).
 */
public final class ResourcesService {

    private final HttpTransport transport;

    public ResourcesService(HttpTransport transport) {
        this.transport = transport;
    }

    public ResourceListResponse list() {
        return list(null, null, null);
    }

    public ResourceListResponse list(Long folderId, String resourceType, String q) {
        QueryParams params = new QueryParams();
        if (folderId != null) {
            params.add("folder_id", folderId);
        }
        if (resourceType != null) {
            params.add("resource_type", resourceType);
        }
        if (q != null) {
            params.add("q", q);
        }
        return transport.getJson("/api/v1/resources", params.isEmpty() ? null : params, ResourceListResponse.class);
    }

    public Resource get(String uuid) {
        return transport.getJson("/api/v1/resources/" + uuid, Resource.class);
    }

    public Resource createJson(Map<String, Object> body) {
        return transport.postJson("/api/v1/resources", body, Resource.class);
    }

    public Resource upload(
            Path file,
            Long folderId,
            String displayName,
            String resourceType) throws IOException {
        MultipartBody body = new MultipartBody();
        body.addFile("file", file, "application/octet-stream");
        if (folderId != null) {
            body.addField("folder_id", String.valueOf(folderId));
        }
        if (displayName != null) {
            body.addField("display_name", displayName);
        }
        if (resourceType != null) {
            body.addField("resource_type", resourceType);
        }
        return transport.postMultipart("/api/v1/resources", body, Resource.class);
    }

    public Resource update(String uuid, Map<String, Object> fields) {
        return transport.putJson("/api/v1/resources/" + uuid, fields, Resource.class);
    }

    public Resource replaceContent(String uuid, Path file) throws IOException {
        MultipartBody body = new MultipartBody();
        body.addFile("file", file, "application/octet-stream");
        return transport.putMultipart("/api/v1/resources/" + uuid + "/content", body, Resource.class);
    }

    public SuccessResponse delete(String uuid) {
        return transport.deleteJson("/api/v1/resources/" + uuid, null, SuccessResponse.class);
    }

    public BinaryResult download(String uuid) {
        return download(uuid, false);
    }

    public BinaryResult download(String uuid, boolean inline) {
        QueryParams q = inline ? new QueryParams().add("inline", "1") : null;
        RawResponse resp = transport.request("GET", "/api/v1/resources/" + uuid + "/download", q, null, null, true);
        return new BinaryResult(resp.body(), resp.contentType().orElse(null));
    }

    /** Download resource bytes to a filesystem path. */
    public Path downloadToFile(String uuid, Path destination) throws IOException {
        transport.downloadTo("/api/v1/resources/" + uuid + "/download", null, destination);
        return destination;
    }

    public byte[] thumbnail(String uuid) {
        return transport.getBytes("/api/v1/resources/" + uuid + "/thumbnail", null);
    }

    // --- images ---

    public ResourceListResponse listImages() {
        return listImages(null, null);
    }

    public ResourceListResponse listImages(Long folderId, String q) {
        QueryParams params = new QueryParams();
        if (folderId != null) {
            params.add("folder_id", folderId);
        }
        if (q != null) {
            params.add("q", q);
        }
        return transport.getJson(
                "/api/v1/resources/images", params.isEmpty() ? null : params, ResourceListResponse.class);
    }

    public Resource getImage(String uuid) {
        return transport.getJson("/api/v1/resources/images/" + uuid, Resource.class);
    }

    public Resource uploadImage(Path file, Long folderId, String displayName) throws IOException {
        MultipartBody body = new MultipartBody();
        body.addFile("file", file, guessImageContentType(file));
        if (folderId != null) {
            body.addField("folder_id", String.valueOf(folderId));
        }
        if (displayName != null) {
            body.addField("display_name", displayName);
        }
        return transport.postMultipart("/api/v1/resources/images", body, Resource.class);
    }

    public Resource replaceImage(String uuid, Path file) throws IOException {
        MultipartBody body = new MultipartBody();
        body.addFile("file", file, guessImageContentType(file));
        return transport.putMultipart("/api/v1/resources/images/" + uuid, body, Resource.class);
    }

    public SuccessResponse deleteImage(String uuid) {
        return transport.deleteJson("/api/v1/resources/images/" + uuid, null, SuccessResponse.class);
    }

    public BinaryResult downloadImage(String uuid) {
        return downloadImage(uuid, false);
    }

    public BinaryResult downloadImage(String uuid, boolean inline) {
        QueryParams q = inline ? new QueryParams().add("inline", "1") : null;
        RawResponse resp =
                transport.request("GET", "/api/v1/resources/images/" + uuid + "/download", q, null, null, true);
        return new BinaryResult(resp.body(), resp.contentType().orElse(null));
    }

    public Path downloadImageToFile(String uuid, Path destination) throws IOException {
        transport.downloadTo("/api/v1/resources/images/" + uuid + "/download", null, destination);
        return destination;
    }

    // --- resource sync ---

    public JsonNode syncHint() {
        return transport.getJsonNode("/api/v1/resource-sync");
    }

    public JsonNode runSync() {
        return transport.postJson("/api/v1/resource-sync", Map.of());
    }

    public Resource createTextResource(
            String displayName, String resourceType, String content, Long folderId) {
        Map<String, Object> body = new HashMap<>();
        body.put("display_name", displayName);
        body.put("resource_type", resourceType);
        body.put("content", content);
        if (folderId != null) {
            body.put("folder_id", folderId);
        }
        return createJson(body);
    }

    private static String guessImageContentType(Path file) {
        String name = file.getFileName().toString().toLowerCase();
        if (name.endsWith(".png")) {
            return "image/png";
        }
        if (name.endsWith(".jpg") || name.endsWith(".jpeg")) {
            return "image/jpeg";
        }
        if (name.endsWith(".gif")) {
            return "image/gif";
        }
        if (name.endsWith(".svg")) {
            return "image/svg+xml";
        }
        if (name.endsWith(".webp")) {
            return "image/webp";
        }
        return "application/octet-stream";
    }
}
