package com.acugis.report;

import com.sun.net.httpserver.Headers;
import com.sun.net.httpserver.HttpExchange;
import com.sun.net.httpserver.HttpServer;

import java.io.IOException;
import java.io.OutputStream;
import java.net.InetSocketAddress;
import java.nio.charset.StandardCharsets;
import java.util.HashMap;
import java.util.Map;
import java.util.concurrent.Executors;
import java.util.function.BiConsumer;

/** Minimal JDK HttpServer fixture for SDK tests. */
final class MockReportServer implements AutoCloseable {

    private final HttpServer server;
    private final Map<String, BiConsumer<HttpExchange, String>> routes = new HashMap<>();

    MockReportServer() throws IOException {
        server = HttpServer.create(new InetSocketAddress("127.0.0.1", 0), 0);
        server.setExecutor(Executors.newCachedThreadPool());
        server.createContext("/", this::dispatch);
        server.start();
    }

    String baseUrl() {
        return "http://127.0.0.1:" + server.getAddress().getPort();
    }

    void on(String methodAndPath, BiConsumer<HttpExchange, String> handler) {
        routes.put(methodAndPath, handler);
    }

    void json(String methodAndPath, int status, String body) {
        on(methodAndPath, (ex, ignored) -> write(ex, status, "application/json", body));
    }

    void bytes(String methodAndPath, int status, String contentType, byte[] body) {
        on(methodAndPath, (ex, ignored) -> writeBytes(ex, status, contentType, body));
    }

    private void dispatch(HttpExchange exchange) throws IOException {
        String method = exchange.getRequestMethod().toUpperCase();
        String path = exchange.getRequestURI().getPath();
        String key = method + " " + path;
        BiConsumer<HttpExchange, String> handler = routes.get(key);
        if (handler == null) {
            write(exchange, 404, "application/json", "{\"error\":\"not found\"}");
            return;
        }
        String body = new String(exchange.getRequestBody().readAllBytes(), StandardCharsets.UTF_8);
        handler.accept(exchange, body);
    }

    static void write(HttpExchange exchange, int status, String contentType, String body) {
        writeBytes(exchange, status, contentType, body.getBytes(StandardCharsets.UTF_8));
    }

    static void writeBytes(HttpExchange exchange, int status, String contentType, byte[] body) {
        try {
            Headers headers = exchange.getResponseHeaders();
            headers.set("Content-Type", contentType);
            exchange.sendResponseHeaders(status, body.length);
            try (OutputStream os = exchange.getResponseBody()) {
                os.write(body);
            }
        } catch (IOException e) {
            throw new RuntimeException(e);
        }
    }

    @Override
    public void close() {
        server.stop(0);
    }
}
