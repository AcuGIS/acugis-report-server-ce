package com.acugis.report.internal;

import com.acugis.report.ClientConfig;
import com.acugis.report.errors.ErrorMapper;
import com.acugis.report.errors.InvalidResponseException;
import com.acugis.report.errors.NetworkException;
import com.acugis.report.errors.TimeoutException;
import com.fasterxml.jackson.core.type.TypeReference;
import com.fasterxml.jackson.databind.JsonNode;

import javax.net.ssl.SSLContext;
import javax.net.ssl.TrustManager;
import javax.net.ssl.X509TrustManager;
import java.io.IOException;
import java.io.InputStream;
import java.io.OutputStream;
import java.net.URI;
import java.net.http.HttpClient;
import java.net.http.HttpRequest;
import java.net.http.HttpResponse;
import java.nio.file.Files;
import java.nio.file.Path;
import java.security.SecureRandom;
import java.security.cert.X509Certificate;
import java.time.Duration;
import java.util.Map;
import java.util.Objects;
import java.util.concurrent.CompletableFuture;
import java.util.concurrent.CompletionException;

/**
 * Low-level HTTP transport using {@link java.net.http.HttpClient}.
 * Supports optional GET-only retries, TLS verification toggle, and streaming binaries.
 */
public final class HttpTransport implements AutoCloseable {

    private final ClientConfig config;
    private final HttpClient httpClient;
    private final boolean ownsClient;

    public HttpTransport(ClientConfig config) {
        this.config = Objects.requireNonNull(config, "config");
        this.httpClient = buildClient(config);
        this.ownsClient = true;
    }

    public HttpTransport(ClientConfig config, HttpClient httpClient) {
        this.config = Objects.requireNonNull(config, "config");
        this.httpClient = Objects.requireNonNull(httpClient, "httpClient");
        this.ownsClient = false;
    }

    public ClientConfig config() {
        return config;
    }

    public HttpClient httpClient() {
        return httpClient;
    }

    public RawResponse request(
            String method,
            String path,
            QueryParams query,
            byte[] body,
            String contentType,
            boolean raiseForStatus) {
        int attempts = 1 + Math.max(0, "GET".equalsIgnoreCase(method) ? config.getMaxGetRetries() : 0);
        IOException lastIo = null;
        InterruptedException lastInterrupted = null;
        for (int attempt = 0; attempt < attempts; attempt++) {
            try {
                if (attempt > 0) {
                    sleepBackoff(attempt);
                }
                RawResponse response = sendOnce(method, path, query, body, contentType);
                if (raiseForStatus && response.statusCode() >= 400) {
                    boolean retryable = "GET".equalsIgnoreCase(method)
                            && response.statusCode() >= 500
                            && attempt + 1 < attempts;
                    if (retryable) {
                        continue;
                    }
                    throw ErrorMapper.fromStatus(response.statusCode(), response.body(), JsonSupport.mapper());
                }
                return response;
            } catch (java.net.http.HttpTimeoutException e) {
                throw new TimeoutException("Request timed out: " + e.getMessage(), e);
            } catch (IOException e) {
                lastIo = e;
                if (attempt + 1 >= attempts || !"GET".equalsIgnoreCase(method)) {
                    throw new NetworkException("Network error: " + e.getMessage(), e);
                }
            } catch (InterruptedException e) {
                lastInterrupted = e;
                Thread.currentThread().interrupt();
                throw new NetworkException("Request interrupted", e);
            }
        }
        if (lastInterrupted != null) {
            throw new NetworkException("Request interrupted", lastInterrupted);
        }
        throw new NetworkException("Network error: " + (lastIo != null ? lastIo.getMessage() : "unknown"), lastIo);
    }

    public CompletableFuture<RawResponse> requestAsync(
            String method,
            String path,
            QueryParams query,
            byte[] body,
            String contentType,
            boolean raiseForStatus) {
        HttpRequest httpRequest = buildRequest(method, path, query, body, contentType);
        CompletableFuture<HttpResponse<byte[]>> future =
                httpClient.sendAsync(httpRequest, HttpResponse.BodyHandlers.ofByteArray());
        return future.handle((resp, error) -> {
            if (error != null) {
                Throwable cause = error instanceof CompletionException && error.getCause() != null
                        ? error.getCause()
                        : error;
                if (cause instanceof java.net.http.HttpTimeoutException) {
                    throw new TimeoutException("Request timed out: " + cause.getMessage(), cause);
                }
                if (cause instanceof IOException) {
                    throw new NetworkException("Network error: " + cause.getMessage(), cause);
                }
                if (cause instanceof RuntimeException re) {
                    throw re;
                }
                throw new NetworkException("Network error: " + cause.getMessage(), cause);
            }
            RawResponse raw = toRaw(resp);
            if (raiseForStatus && raw.statusCode() >= 400) {
                throw ErrorMapper.fromStatus(raw.statusCode(), raw.body(), JsonSupport.mapper());
            }
            return raw;
        });
    }

    public <T> T getJson(String path, QueryParams query, Class<T> type) {
        RawResponse resp = request("GET", path, query, null, null, true);
        return decodeJson(resp, type);
    }

    public <T> T getJson(String path, Class<T> type) {
        return getJson(path, null, type);
    }

    public <T> T getJson(String path, TypeReference<T> type) {
        return getJson(path, null, type);
    }

    public <T> T getJson(String path, QueryParams query, TypeReference<T> type) {
        RawResponse resp = request("GET", path, query, null, null, true);
        return decodeJson(resp, type);
    }

    public JsonNode getJsonNode(String path, QueryParams query) {
        RawResponse resp = request("GET", path, query, null, null, true);
        return decodeJsonNode(resp);
    }

    public JsonNode getJsonNode(String path) {
        return getJsonNode(path, null);
    }

    public <T> T postJson(String path, Object body, Class<T> type) {
        byte[] bytes = body == null ? new byte[0] : JsonSupport.writeBytes(body);
        RawResponse resp = request("POST", path, null, bytes, "application/json", true);
        return decodeJson(resp, type);
    }

    public JsonNode postJson(String path, Object body) {
        byte[] bytes = body == null ? new byte[0] : JsonSupport.writeBytes(body);
        RawResponse resp = request("POST", path, null, bytes, "application/json", true);
        return decodeJsonNode(resp);
    }

    public <T> T putJson(String path, Object body, Class<T> type) {
        byte[] bytes = body == null ? new byte[0] : JsonSupport.writeBytes(body);
        RawResponse resp = request("PUT", path, null, bytes, "application/json", true);
        return decodeJson(resp, type);
    }

    public JsonNode putJson(String path, Object body) {
        byte[] bytes = body == null ? new byte[0] : JsonSupport.writeBytes(body);
        RawResponse resp = request("PUT", path, null, bytes, "application/json", true);
        return decodeJsonNode(resp);
    }

    public <T> T deleteJson(String path, QueryParams query, Class<T> type) {
        RawResponse resp = request("DELETE", path, query, null, null, true);
        if (resp.statusCode() == 204 || resp.body().length == 0) {
            return null;
        }
        return decodeJson(resp, type);
    }

    public JsonNode deleteJson(String path, QueryParams query) {
        RawResponse resp = request("DELETE", path, query, null, null, true);
        if (resp.statusCode() == 204 || resp.body().length == 0) {
            return JsonSupport.mapper().createObjectNode();
        }
        return decodeJsonNode(resp);
    }

    public <T> T postForm(String path, QueryParams form, Class<T> type) {
        byte[] bytes = form.encode().getBytes(java.nio.charset.StandardCharsets.UTF_8);
        RawResponse resp = request("POST", path, null, bytes, "application/x-www-form-urlencoded", true);
        return decodeJson(resp, type);
    }

    public JsonNode postForm(String path, QueryParams form) {
        byte[] bytes = form.encode().getBytes(java.nio.charset.StandardCharsets.UTF_8);
        RawResponse resp = request("POST", path, null, bytes, "application/x-www-form-urlencoded", true);
        return decodeJsonNode(resp);
    }

    public <T> T postMultipart(String path, MultipartBody multipart, Class<T> type) {
        RawResponse resp = request("POST", path, null, multipart.build(), multipart.contentType(), true);
        return decodeJson(resp, type);
    }

    public JsonNode postMultipart(String path, MultipartBody multipart) {
        RawResponse resp = request("POST", path, null, multipart.build(), multipart.contentType(), true);
        return decodeJsonNode(resp);
    }

    public <T> T putMultipart(String path, MultipartBody multipart, Class<T> type) {
        RawResponse resp = request("PUT", path, null, multipart.build(), multipart.contentType(), true);
        return decodeJson(resp, type);
    }

    public JsonNode putMultipart(String path, MultipartBody multipart) {
        RawResponse resp = request("PUT", path, null, multipart.build(), multipart.contentType(), true);
        return decodeJsonNode(resp);
    }

    public byte[] getBytes(String path, QueryParams query) {
        return request("GET", path, query, null, null, true).body();
    }

    public void downloadTo(String path, QueryParams query, Path destination) throws IOException {
        RawResponse resp = request("GET", path, query, null, null, true);
        if (destination.getParent() != null) {
            Files.createDirectories(destination.getParent());
        }
        Files.write(destination, resp.body());
    }

    public void downloadToStream(String path, QueryParams query, OutputStream out) throws IOException {
        RawResponse resp = request("GET", path, query, null, null, true);
        out.write(resp.body());
    }

    public InputStream openStream(String path, QueryParams query) {
        RawResponse resp = request("GET", path, query, null, null, true);
        return new java.io.ByteArrayInputStream(resp.body());
    }

    public <T> T decodeJson(RawResponse resp, Class<T> type) {
        if (resp.statusCode() == 204 || resp.body().length == 0) {
            return null;
        }
        try {
            return JsonSupport.read(resp.body(), type);
        } catch (IOException e) {
            throw new InvalidResponseException(
                    "Invalid JSON response", resp.statusCode(), resp.bodyAsString(), e);
        }
    }

    public <T> T decodeJson(RawResponse resp, TypeReference<T> type) {
        if (resp.statusCode() == 204 || resp.body().length == 0) {
            return null;
        }
        try {
            return JsonSupport.mapper().readValue(resp.body(), type);
        } catch (IOException e) {
            throw new InvalidResponseException(
                    "Invalid JSON response", resp.statusCode(), resp.bodyAsString(), e);
        }
    }

    public JsonNode decodeJsonNode(RawResponse resp) {
        if (resp.statusCode() == 204 || resp.body().length == 0) {
            return JsonSupport.mapper().createObjectNode();
        }
        try {
            return JsonSupport.readTree(resp.body());
        } catch (IOException e) {
            throw new InvalidResponseException(
                    "Invalid JSON response", resp.statusCode(), resp.bodyAsString(), e);
        }
    }

    @Override
    public void close() {
        // java.net.http.HttpClient does not require explicit close in Java 17.
        // Reserved for future resource cleanup when ownsClient is true.
    }

    private RawResponse sendOnce(
            String method, String path, QueryParams query, byte[] body, String contentType)
            throws IOException, InterruptedException {
        HttpRequest request = buildRequest(method, path, query, body, contentType);
        HttpResponse<byte[]> response = httpClient.send(request, HttpResponse.BodyHandlers.ofByteArray());
        return toRaw(response);
    }

    private HttpRequest buildRequest(
            String method, String path, QueryParams query, byte[] body, String contentType) {
        URI uri = resolve(path, query);
        HttpRequest.Builder builder = HttpRequest.newBuilder(uri)
                .timeout(config.getTimeout())
                .header("Accept", "application/json")
                .header("User-Agent", config.getUserAgent());

        if (config.getToken() != null && !config.getToken().isBlank()) {
            builder.header("Authorization", "Bearer " + config.getToken().trim());
        }
        for (Map.Entry<String, String> header : config.getDefaultHeaders().entrySet()) {
            if (header.getKey() != null && header.getValue() != null) {
                builder.header(header.getKey(), header.getValue());
            }
        }

        String m = method.toUpperCase();
        if ("GET".equals(m) || "DELETE".equals(m) || "HEAD".equals(m)) {
            builder.method(m, HttpRequest.BodyPublishers.noBody());
        } else {
            byte[] payload = body == null ? new byte[0] : body;
            if (contentType != null) {
                builder.header("Content-Type", contentType);
            }
            builder.method(m, HttpRequest.BodyPublishers.ofByteArray(payload));
        }
        return builder.build();
    }

    private URI resolve(String path, QueryParams query) {
        String base = config.getBaseUrl().replaceAll("/+$", "");
        String p = path.startsWith("/") ? path : "/" + path;
        String url = base + p;
        if (query != null && !query.isEmpty()) {
            url = url + "?" + query.encode();
        }
        return URI.create(url);
    }

    private static RawResponse toRaw(HttpResponse<byte[]> response) {
        String contentType = response.headers().firstValue("Content-Type").orElse(null);
        return new RawResponse(response.statusCode(), response.body(), contentType, response.headers());
    }

    private void sleepBackoff(int attempt) {
        Duration base = config.getRetryBackoff();
        long millis = base.toMillis() * (1L << Math.min(attempt - 1, 4));
        try {
            Thread.sleep(millis);
        } catch (InterruptedException e) {
            Thread.currentThread().interrupt();
            throw new NetworkException("Retry interrupted", e);
        }
    }

    private static HttpClient buildClient(ClientConfig config) {
        HttpClient.Builder builder = HttpClient.newBuilder()
                .connectTimeout(config.getTimeout())
                .followRedirects(HttpClient.Redirect.NORMAL);
        if (!config.isVerifyTls()) {
            builder.sslContext(insecureSslContext());
        }
        return builder.build();
    }

    private static SSLContext insecureSslContext() {
        try {
            TrustManager[] trustAll = new TrustManager[] {
                new X509TrustManager() {
                    @Override
                    public void checkClientTrusted(X509Certificate[] chain, String authType) {}

                    @Override
                    public void checkServerTrusted(X509Certificate[] chain, String authType) {}

                    @Override
                    public X509Certificate[] getAcceptedIssuers() {
                        return new X509Certificate[0];
                    }
                }
            };
            SSLContext ctx = SSLContext.getInstance("TLS");
            ctx.init(null, trustAll, new SecureRandom());
            return ctx;
        } catch (Exception e) {
            throw new IllegalStateException("Failed to create insecure SSL context", e);
        }
    }
}
