package com.acugis.report.internal;

import java.io.ByteArrayOutputStream;
import java.io.IOException;
import java.nio.charset.StandardCharsets;
import java.nio.file.Files;
import java.nio.file.Path;
import java.util.ArrayList;
import java.util.List;
import java.util.UUID;

/** Minimal multipart/form-data body builder for java.net.http.HttpClient. */
public final class MultipartBody {

    private final String boundary = "----AcuGISReportBoundary" + UUID.randomUUID().toString().replace("-", "");
    private final List<Part> parts = new ArrayList<>();

    public MultipartBody addField(String name, String value) {
        if (value != null) {
            parts.add(Part.field(name, value));
        }
        return this;
    }

    public MultipartBody addFile(String name, Path path, String contentType) throws IOException {
        byte[] bytes = Files.readAllBytes(path);
        String filename = path.getFileName().toString();
        parts.add(Part.file(name, filename, contentType == null ? "application/octet-stream" : contentType, bytes));
        return this;
    }

    public MultipartBody addFile(String name, String filename, String contentType, byte[] bytes) {
        parts.add(Part.file(name, filename, contentType == null ? "application/octet-stream" : contentType, bytes));
        return this;
    }

    public String contentType() {
        return "multipart/form-data; boundary=" + boundary;
    }

    public byte[] build() {
        ByteArrayOutputStream out = new ByteArrayOutputStream();
        byte[] crlf = "\r\n".getBytes(StandardCharsets.UTF_8);
        try {
            for (Part part : parts) {
                out.write(("--" + boundary).getBytes(StandardCharsets.UTF_8));
                out.write(crlf);
                if (part.filename == null) {
                    out.write(("Content-Disposition: form-data; name=\"" + part.name + "\"")
                            .getBytes(StandardCharsets.UTF_8));
                    out.write(crlf);
                    out.write(crlf);
                    out.write(part.data);
                    out.write(crlf);
                } else {
                    out.write(("Content-Disposition: form-data; name=\"" + part.name
                                    + "\"; filename=\"" + part.filename + "\"")
                            .getBytes(StandardCharsets.UTF_8));
                    out.write(crlf);
                    out.write(("Content-Type: " + part.contentType).getBytes(StandardCharsets.UTF_8));
                    out.write(crlf);
                    out.write(crlf);
                    out.write(part.data);
                    out.write(crlf);
                }
            }
            out.write(("--" + boundary + "--").getBytes(StandardCharsets.UTF_8));
            out.write(crlf);
        } catch (IOException e) {
            throw new IllegalStateException("Failed to build multipart body", e);
        }
        return out.toByteArray();
    }

    public boolean isEmpty() {
        return parts.isEmpty();
    }

    private record Part(String name, String filename, String contentType, byte[] data) {
        static Part field(String name, String value) {
            return new Part(name, null, null, value.getBytes(StandardCharsets.UTF_8));
        }

        static Part file(String name, String filename, String contentType, byte[] data) {
            return new Part(name, filename, contentType, data);
        }
    }
}
