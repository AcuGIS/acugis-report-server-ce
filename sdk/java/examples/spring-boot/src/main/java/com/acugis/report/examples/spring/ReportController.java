package com.acugis.report.examples.spring;

import com.acugis.report.ReportServerClient;
import com.acugis.report.models.BinaryResult;
import com.acugis.report.models.ReportListResponse;
import com.acugis.report.models.User;
import org.springframework.http.HttpHeaders;
import org.springframework.http.MediaType;
import org.springframework.http.ResponseEntity;
import org.springframework.web.bind.annotation.GetMapping;
import org.springframework.web.bind.annotation.PathVariable;
import org.springframework.web.bind.annotation.RequestMapping;
import org.springframework.web.bind.annotation.RequestParam;
import org.springframework.web.bind.annotation.RestController;

import java.util.HashMap;
import java.util.Map;

@RestController
@RequestMapping("/api")
public class ReportController {

    private final ReportServerClient client;

    public ReportController(ReportServerClient client) {
        this.client = client;
    }

    @GetMapping("/me")
    public User me() {
        return client.auth().me();
    }

    @GetMapping("/reports")
    public ReportListResponse reports() {
        return client.reports().list();
    }

    @GetMapping("/reports/{key}/pdf")
    public ResponseEntity<byte[]> runPdf(
            @PathVariable String key, @RequestParam Map<String, String> params) {
        Map<String, Object> reportParams = new HashMap<>(params);
        BinaryResult result = client.reports().run(key, reportParams, "pdf");
        return ResponseEntity.ok()
                .header(HttpHeaders.CONTENT_DISPOSITION, "inline; filename=\"" + key + ".pdf\"")
                .contentType(MediaType.APPLICATION_PDF)
                .body(result.getContent());
    }
}
