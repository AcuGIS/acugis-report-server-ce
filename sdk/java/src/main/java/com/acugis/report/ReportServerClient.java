package com.acugis.report;

import com.acugis.report.errors.NotImplementedByServerException;
import com.acugis.report.internal.HttpTransport;
import com.acugis.report.services.AuthService;
import com.acugis.report.services.DatasourcesService;
import com.acugis.report.services.EmbeddedMapsService;
import com.acugis.report.services.FoldersService;
import com.acugis.report.services.HealthService;
import com.acugis.report.services.MapReportsService;
import com.acugis.report.services.ReportsService;
import com.acugis.report.services.ResourcesService;

import java.time.Duration;
import java.util.Map;

/**
 * Official Java client for AcuGIS Report Server.
 *
 * <pre>{@code
 * ReportServerClient client = ReportServerClient.builder()
 *     .baseUrl("https://reports.example.com")
 *     .token(token)
 *     .timeout(Duration.ofSeconds(60))
 *     .verifyTls(true)
 *     .header("X-Custom", "1")
 *     .build();
 *
 * client.reports().list();
 * client.reports().runToFile("Invoice", Map.of("InvoiceID", 123), Path.of("invoice.pdf"));
 * }</pre>
 */
public final class ReportServerClient implements AutoCloseable {

    private final ClientConfig config;
    private final HttpTransport transport;
    private final AuthService auth;
    private final ReportsService reports;
    private final FoldersService folders;
    private final ResourcesService resources;
    private final DatasourcesService datasources;
    private final MapReportsService mapReports;
    private final EmbeddedMapsService embeddedMaps;
    private final HealthService health;

    private ReportServerClient(ClientConfig config, HttpTransport transport) {
        this.config = config;
        this.transport = transport;
        this.auth = new AuthService(transport);
        this.reports = new ReportsService(transport);
        this.folders = new FoldersService(transport);
        this.resources = new ResourcesService(transport);
        this.datasources = new DatasourcesService(transport);
        this.mapReports = new MapReportsService(transport, reports);
        this.embeddedMaps = new EmbeddedMapsService(transport);
        this.health = new HealthService(transport);
    }

    public static Builder builder() {
        return new Builder();
    }

    /** Create a client from {@code ACUGIS_REPORT_URL} / {@code ACUGIS_REPORT_TOKEN}. */
    public static ReportServerClient fromEnvironment() {
        ClientConfig config = ClientConfig.fromEnvironment();
        return new ReportServerClient(config, new HttpTransport(config));
    }

    public ClientConfig config() {
        return config;
    }

    /** Low-level HTTP transport (advanced use). */
    public HttpTransport transport() {
        return transport;
    }

    public AuthService auth() {
        return auth;
    }

    public ReportsService reports() {
        return reports;
    }

    public FoldersService folders() {
        return folders;
    }

    public ResourcesService resources() {
        return resources;
    }

    public DatasourcesService datasources() {
        return datasources;
    }

    public MapReportsService mapReports() {
        return mapReports;
    }

    public EmbeddedMapsService embeddedMaps() {
        return embeddedMaps;
    }

    public HealthService health() {
        return health;
    }

    /** Reserved — not yet implemented by Report Server REST API. */
    public Object users() {
        throw new NotImplementedByServerException("users");
    }

    /** Reserved — not yet implemented by Report Server REST API. */
    public Object groups() {
        throw new NotImplementedByServerException("groups");
    }

    /** Reserved — not yet implemented by Report Server REST API. */
    public Object schedules() {
        throw new NotImplementedByServerException("schedules");
    }

    /** Reserved — not yet implemented by Report Server REST API. */
    public Object jobs() {
        throw new NotImplementedByServerException("jobs");
    }

    /** Reserved — not yet implemented by Report Server REST API. */
    public Object recipients() {
        throw new NotImplementedByServerException("recipients");
    }

    @Override
    public void close() {
        transport.close();
    }

    public static final class Builder {
        private final ClientConfig.Builder configBuilder = ClientConfig.builder();
        private java.net.http.HttpClient httpClient;

        public Builder baseUrl(String baseUrl) {
            configBuilder.baseUrl(baseUrl);
            return this;
        }

        public Builder token(String token) {
            configBuilder.token(token);
            return this;
        }

        public Builder timeout(Duration timeout) {
            configBuilder.timeout(timeout);
            return this;
        }

        public Builder verifyTls(boolean verifyTls) {
            configBuilder.verifyTls(verifyTls);
            return this;
        }

        public Builder header(String name, String value) {
            configBuilder.header(name, value);
            return this;
        }

        public Builder headers(Map<String, String> headers) {
            configBuilder.headers(headers);
            return this;
        }

        public Builder userAgent(String userAgent) {
            configBuilder.userAgent(userAgent);
            return this;
        }

        public Builder maxGetRetries(int maxGetRetries) {
            configBuilder.maxGetRetries(maxGetRetries);
            return this;
        }

        public Builder retryBackoff(Duration retryBackoff) {
            configBuilder.retryBackoff(retryBackoff);
            return this;
        }

        /** Load URL/token from {@code ACUGIS_REPORT_URL} / {@code ACUGIS_REPORT_TOKEN}. */
        public Builder fromEnvironment() {
            configBuilder.fromEnvironment();
            return this;
        }

        /** Inject a custom {@link java.net.http.HttpClient} (tests / advanced TLS). */
        public Builder httpClient(java.net.http.HttpClient httpClient) {
            this.httpClient = httpClient;
            return this;
        }

        public ReportServerClient build() {
            ClientConfig config = configBuilder.build();
            HttpTransport transport = httpClient == null
                    ? new HttpTransport(config)
                    : new HttpTransport(config, httpClient);
            return new ReportServerClient(config, transport);
        }
    }
}
