package com.acugis.report.examples.spring;

import com.acugis.report.ClientConfig;
import com.acugis.report.ReportServerClient;
import org.springframework.beans.factory.annotation.Value;
import org.springframework.context.annotation.Bean;
import org.springframework.context.annotation.Configuration;

import java.time.Duration;

@Configuration
public class ReportClientConfig {

    @Bean(destroyMethod = "close")
    public ReportServerClient reportServerClient(
            @Value("${acugis.report.url:}") String url,
            @Value("${acugis.report.token:}") String token) {
        ReportServerClient.Builder builder = ReportServerClient.builder()
                .timeout(Duration.ofSeconds(60));

        String resolvedUrl = blank(url) ? System.getenv(ClientConfig.ENV_URL) : url;
        String resolvedToken = blank(token) ? System.getenv(ClientConfig.ENV_TOKEN) : token;

        if (blank(resolvedUrl)) {
            throw new IllegalStateException(
                    "Set acugis.report.url or environment variable " + ClientConfig.ENV_URL);
        }
        builder.baseUrl(resolvedUrl);
        if (!blank(resolvedToken)) {
            builder.token(resolvedToken);
        }
        return builder.build();
    }

    private static boolean blank(String s) {
        return s == null || s.isBlank();
    }
}
