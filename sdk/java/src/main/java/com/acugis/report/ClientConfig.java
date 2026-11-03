package com.acugis.report;

import java.time.Duration;
import java.util.Collections;
import java.util.LinkedHashMap;
import java.util.Map;
import java.util.Objects;

/**
 * Immutable configuration for {@link ReportServerClient}.
 */
public final class ClientConfig {

    /** Environment variable for the report server base URL. */
    public static final String ENV_URL = "ACUGIS_REPORT_URL";

    /** Environment variable for the personal access token. */
    public static final String ENV_TOKEN = "ACUGIS_REPORT_TOKEN";

    /** SDK User-Agent header value. */
    public static final String DEFAULT_USER_AGENT = "AcuGIS-Report-Java/0.1.0";

    private final String baseUrl;
    private final String token;
    private final Duration timeout;
    private final boolean verifyTls;
    private final Map<String, String> defaultHeaders;
    private final String userAgent;
    private final int maxGetRetries;
    private final Duration retryBackoff;

    private ClientConfig(Builder builder) {
        this.baseUrl = trimTrailingSlash(Objects.requireNonNull(builder.baseUrl, "baseUrl").trim());
        if (this.baseUrl.isEmpty()) {
            throw new IllegalArgumentException("baseUrl is required");
        }
        this.token = builder.token == null || builder.token.isBlank() ? null : builder.token.trim();
        this.timeout = builder.timeout == null ? Duration.ofSeconds(60) : builder.timeout;
        this.verifyTls = builder.verifyTls;
        this.defaultHeaders = Collections.unmodifiableMap(new LinkedHashMap<>(builder.defaultHeaders));
        this.userAgent = builder.userAgent == null || builder.userAgent.isBlank()
                ? DEFAULT_USER_AGENT
                : builder.userAgent;
        this.maxGetRetries = Math.max(0, builder.maxGetRetries);
        this.retryBackoff = builder.retryBackoff == null ? Duration.ofMillis(200) : builder.retryBackoff;
    }

    /**
     * Load base URL and token from {@code ACUGIS_REPORT_URL} / {@code ACUGIS_REPORT_TOKEN}.
     */
    public static ClientConfig fromEnvironment() {
        String url = System.getenv(ENV_URL);
        String token = System.getenv(ENV_TOKEN);
        if (url == null || url.isBlank()) {
            throw new IllegalStateException("Environment variable " + ENV_URL + " is required");
        }
        return builder().baseUrl(url).token(token).build();
    }

    public static Builder builder() {
        return new Builder();
    }

    public String getBaseUrl() {
        return baseUrl;
    }

    public String getToken() {
        return token;
    }

    public Duration getTimeout() {
        return timeout;
    }

    public boolean isVerifyTls() {
        return verifyTls;
    }

    public Map<String, String> getDefaultHeaders() {
        return defaultHeaders;
    }

    public String getUserAgent() {
        return userAgent;
    }

    public int getMaxGetRetries() {
        return maxGetRetries;
    }

    public Duration getRetryBackoff() {
        return retryBackoff;
    }

    public Builder toBuilder() {
        Builder b = new Builder()
                .baseUrl(baseUrl)
                .token(token)
                .timeout(timeout)
                .verifyTls(verifyTls)
                .userAgent(userAgent)
                .maxGetRetries(maxGetRetries)
                .retryBackoff(retryBackoff);
        defaultHeaders.forEach(b::header);
        return b;
    }

    private static String trimTrailingSlash(String url) {
        String s = url;
        while (s.endsWith("/")) {
            s = s.substring(0, s.length() - 1);
        }
        return s;
    }

    public static final class Builder {
        private String baseUrl;
        private String token;
        private Duration timeout = Duration.ofSeconds(60);
        private boolean verifyTls = true;
        private final Map<String, String> defaultHeaders = new LinkedHashMap<>();
        private String userAgent = DEFAULT_USER_AGENT;
        private int maxGetRetries = 0;
        private Duration retryBackoff = Duration.ofMillis(200);

        public Builder baseUrl(String baseUrl) {
            this.baseUrl = baseUrl;
            return this;
        }

        public Builder token(String token) {
            this.token = token;
            return this;
        }

        public Builder timeout(Duration timeout) {
            this.timeout = timeout;
            return this;
        }

        public Builder verifyTls(boolean verifyTls) {
            this.verifyTls = verifyTls;
            return this;
        }

        public Builder header(String name, String value) {
            if (name != null && value != null) {
                this.defaultHeaders.put(name, value);
            }
            return this;
        }

        public Builder headers(Map<String, String> headers) {
            if (headers != null) {
                this.defaultHeaders.putAll(headers);
            }
            return this;
        }

        public Builder userAgent(String userAgent) {
            this.userAgent = userAgent;
            return this;
        }

        /**
         * When greater than 0, failed GET requests are retried this many times
         * (exponential backoff). Mutations are never retried.
         */
        public Builder maxGetRetries(int maxGetRetries) {
            this.maxGetRetries = maxGetRetries;
            return this;
        }

        public Builder retryBackoff(Duration retryBackoff) {
            this.retryBackoff = retryBackoff;
            return this;
        }

        /** Apply URL/token from environment variables when not already set. */
        public Builder fromEnvironment() {
            if (baseUrl == null || baseUrl.isBlank()) {
                String url = System.getenv(ENV_URL);
                if (url != null && !url.isBlank()) {
                    this.baseUrl = url;
                }
            }
            if (token == null || token.isBlank()) {
                String envToken = System.getenv(ENV_TOKEN);
                if (envToken != null && !envToken.isBlank()) {
                    this.token = envToken;
                }
            }
            return this;
        }

        public ClientConfig build() {
            return new ClientConfig(this);
        }
    }
}
