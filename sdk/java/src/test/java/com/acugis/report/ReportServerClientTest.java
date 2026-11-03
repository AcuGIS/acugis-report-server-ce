package com.acugis.report;

import com.acugis.report.errors.AuthenticationException;
import com.acugis.report.errors.NotImplementedByServerException;
import com.acugis.report.errors.ValidationException;
import com.acugis.report.models.BinaryResult;
import com.acugis.report.models.ReportListResponse;
import com.acugis.report.models.User;
import org.junit.jupiter.api.Test;

import java.nio.charset.StandardCharsets;
import java.nio.file.Files;
import java.nio.file.Path;
import java.time.Duration;
import java.util.List;
import java.util.Map;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertNotNull;
import static org.junit.jupiter.api.Assertions.assertThrows;
import static org.junit.jupiter.api.Assertions.assertTrue;

class ReportServerClientTest {

    @Test
    void meAndListReports() throws Exception {
        try (MockReportServer server = new MockReportServer()) {
            server.json(
                    "GET /api/v1/auth/me",
                    200,
                    "{\"id\":1,\"username\":\"admin\",\"is_admin\":true,\"groups\":[\"Administrators\"]}");
            server.json(
                    "GET /api/v1/reports",
                    200,
                    "{\"count\":1,\"items\":[{\"key\":\"Invoice\",\"name\":\"Invoice\",\"type\":\"jasper\"}]}");

            try (ReportServerClient client = ReportServerClient.builder()
                    .baseUrl(server.baseUrl())
                    .token("jrs_test")
                    .timeout(Duration.ofSeconds(5))
                    .build()) {
                User me = client.auth().me();
                assertEquals("admin", me.getUsername());
                assertEquals(true, me.getIsAdmin());

                ReportListResponse list = client.reports().list();
                assertEquals(1, list.getCount());
                assertEquals("Invoice", list.getItems().get(0).getKey());
            }
        }
    }

    @Test
    void runReportWithMultiValueParamsAndSaveFile() throws Exception {
        try (MockReportServer server = new MockReportServer()) {
            server.on("GET /reports/execute", (ex, body) -> {
                String query = ex.getRequestURI().getRawQuery();
                assertTrue(query.contains("_repName=Invoice"));
                assertTrue(query.contains("_repFormat=pdf"));
                if (query.contains("Tags=")) {
                    assertTrue(query.contains("Tags=a") && query.contains("Tags=b"), query);
                }
                MockReportServer.writeBytes(
                        ex, 200, "application/pdf", "%PDF-1.4 fake".getBytes(StandardCharsets.UTF_8));
            });

            Path temp = Files.createTempFile("invoice", ".pdf");
            try (ReportServerClient client = ReportServerClient.builder()
                    .baseUrl(server.baseUrl())
                    .token("jrs_test")
                    .timeout(Duration.ofSeconds(5))
                    .build()) {
                BinaryResult result = client.reports()
                        .run("Invoice", Map.of("InvoiceID", 123, "Tags", List.of("a", "b")), "pdf");
                assertEquals("application/pdf", result.getContentType().orElse(null));
                assertTrue(new String(result.getContent(), StandardCharsets.UTF_8).startsWith("%PDF"));

                client.reports().runToFile("Invoice", Map.of("InvoiceID", 123), temp, "pdf");
                assertTrue(Files.size(temp) > 0);
            } finally {
                Files.deleteIfExists(temp);
            }
        }
    }

    @Test
    void authErrorMapsToTypedException() throws Exception {
        try (MockReportServer server = new MockReportServer()) {
            server.json("GET /api/v1/auth/me", 401, "{\"error\":\"authentication required\"}");
            try (ReportServerClient client = ReportServerClient.builder()
                    .baseUrl(server.baseUrl())
                    .token("bad")
                    .build()) {
                AuthenticationException ex =
                        assertThrows(AuthenticationException.class, () -> client.auth().me());
                assertEquals(401, ex.getStatusCode());
                assertEquals("authentication required", ex.getMessage());
            }
        }
    }

    @Test
    void reservedServicesThrow() {
        try (ReportServerClient client = ReportServerClient.builder()
                .baseUrl("http://127.0.0.1:9")
                .token("x")
                .build()) {
            assertThrows(NotImplementedByServerException.class, client::users);
            assertThrows(NotImplementedByServerException.class, client::groups);
            assertThrows(NotImplementedByServerException.class, client::schedules);
            assertThrows(NotImplementedByServerException.class, client::jobs);
            assertThrows(NotImplementedByServerException.class, client::recipients);
        }
    }

    @Test
    void invalidReportFormatRejected() {
        try (ReportServerClient client = ReportServerClient.builder()
                .baseUrl("http://127.0.0.1:9")
                .token("x")
                .build()) {
            assertThrows(
                    ValidationException.class,
                    () -> client.reports().run("x", Map.of(), "exe"));
        }
    }

    @Test
    void foldersDuplicateAndFavorites() throws Exception {
        try (MockReportServer server = new MockReportServer()) {
            server.json(
                    "POST /api/v1/folders/12/duplicate",
                    201,
                    "{\"id\":99,\"name\":\"Finance copy\",\"parent_id\":0}");
            server.json(
                    "POST /api/v1/favorites",
                    200,
                    "{\"ok\":true,\"favorite\":true}");
            server.json(
                    "POST /api/v1/content/move",
                    200,
                    "{\"content_key\":\"nycdata\",\"content_type\":\"jasper_report\",\"folder_id\":12}");

            try (ReportServerClient client = ReportServerClient.builder()
                    .baseUrl(server.baseUrl())
                    .token("jrs_test")
                    .build()) {
                assertEquals(99L, client.folders().duplicate(12).getId());
                assertNotNull(client.folders().setFavorite("jasper_report", "nycdata", true));
                assertEquals(
                        "nycdata",
                        client.folders().moveContent(12, "jasper_report", "nycdata").getContentKey());
            }
        }
    }

    @Test
    void customHeaderAndUserAgentSent() throws Exception {
        try (MockReportServer server = new MockReportServer()) {
            server.on("GET /health", (ex, body) -> {
                assertEquals("AcuGIS-Report-Java/0.1.0", ex.getRequestHeaders().getFirst("User-Agent"));
                assertEquals("1", ex.getRequestHeaders().getFirst("X-Custom"));
                assertEquals("Bearer jrs_test", ex.getRequestHeaders().getFirst("Authorization"));
                MockReportServer.write(ex, 200, "application/json", "{\"status\":\"ok\"}");
            });
            try (ReportServerClient client = ReportServerClient.builder()
                    .baseUrl(server.baseUrl())
                    .token("jrs_test")
                    .header("X-Custom", "1")
                    .build()) {
                assertEquals("ok", client.health().health().getStatus());
            }
        }
    }

    @Test
    void asyncListAndMe() throws Exception {
        try (MockReportServer server = new MockReportServer()) {
            server.json(
                    "GET /api/v1/auth/me",
                    200,
                    "{\"username\":\"admin\"}");
            server.json(
                    "GET /api/v1/reports",
                    200,
                    "{\"count\":0,\"items\":[]}");
            try (ReportServerClient client = ReportServerClient.builder()
                    .baseUrl(server.baseUrl())
                    .token("jrs_test")
                    .build()) {
                assertEquals("admin", client.auth().meAsync().join().getUsername());
                assertEquals(0, client.reports().listAsync().join().getCount());
            }
        }
    }
}
