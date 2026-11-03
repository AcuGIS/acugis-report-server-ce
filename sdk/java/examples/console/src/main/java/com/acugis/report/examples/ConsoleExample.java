package com.acugis.report.examples;

import com.acugis.report.ReportServerClient;
import com.acugis.report.models.ReportListResponse;
import com.acugis.report.models.ReportSummary;
import com.acugis.report.models.User;

import java.nio.file.Path;
import java.time.Duration;
import java.util.Map;

/**
 * Console example using {@code ACUGIS_REPORT_URL} and {@code ACUGIS_REPORT_TOKEN}.
 *
 * <pre>
 * export ACUGIS_REPORT_URL=https://reports.example.com
 * export ACUGIS_REPORT_TOKEN=jrs_...
 * java -cp ... com.acugis.report.examples.ConsoleExample [reportKey] [out.pdf]
 * </pre>
 */
public final class ConsoleExample {

    public static void main(String[] args) throws Exception {
        try (ReportServerClient client = ReportServerClient.builder()
                .fromEnvironment()
                .timeout(Duration.ofSeconds(60))
                .build()) {

            User me = client.auth().me();
            System.out.println("Authenticated as: " + me.getUsername());

            ReportListResponse reports = client.reports().list();
            System.out.println("Reports (" + reports.getCount() + "):");
            for (ReportSummary r : reports.getItems()) {
                System.out.println("  - " + r.getKey() + " (" + r.getName() + ")");
            }

            if (args.length >= 1) {
                String key = args[0];
                Path out = Path.of(args.length >= 2 ? args[1] : key + ".pdf");
                client.reports().runToFile(key, Map.of(), out, "pdf");
                System.out.println("Wrote " + out.toAbsolutePath());
            }
        }
    }

    private ConsoleExample() {}
}
