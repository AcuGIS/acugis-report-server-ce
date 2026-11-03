package com.acugis.report.errors;

import com.fasterxml.jackson.databind.JsonNode;
import com.fasterxml.jackson.databind.ObjectMapper;

import java.util.ArrayList;
import java.util.List;

/** Maps HTTP status codes and response bodies to typed SDK exceptions. */
public final class ErrorMapper {

    private ErrorMapper() {}

    public static ReportServerException fromStatus(int statusCode, byte[] body, ObjectMapper mapper) {
        String text = body == null || body.length == 0 ? "" : new String(body, java.nio.charset.StandardCharsets.UTF_8);
        ParsedError parsed = parse(text, mapper);

        if (statusCode == 401) {
            return new AuthenticationException(parsed.message, statusCode, text, parsed.code, parsed.hint);
        }
        if (statusCode == 403) {
            return new PermissionDeniedException(parsed.message, statusCode, text, parsed.code, parsed.hint);
        }
        if (statusCode == 404) {
            return new NotFoundException(parsed.message, statusCode, text, parsed.code, parsed.hint);
        }
        if (statusCode == 409) {
            return new ConflictException(parsed.message, statusCode, text, parsed.code, parsed.hint);
        }
        if (statusCode == 400 || statusCode == 422) {
            return new ValidationException(
                    parsed.message, statusCode, text, parsed.code, parsed.hint, parsed.errors);
        }
        if (statusCode >= 500) {
            return new ServerException(parsed.message, statusCode, text, parsed.code, parsed.hint);
        }
        return new ReportServerException(parsed.message, statusCode, text, parsed.code, parsed.hint, null);
    }

    private static ParsedError parse(String text, ObjectMapper mapper) {
        ParsedError result = new ParsedError();
        result.message = text == null || text.isBlank() ? "HTTP error" : truncate(text.trim(), 500);
        if (text == null || text.isBlank()) {
            return result;
        }
        String trimmed = text.trim();
        if (!(trimmed.startsWith("{") || trimmed.startsWith("["))) {
            return result;
        }
        try {
            JsonNode node = mapper.readTree(trimmed);
            if (node.hasNonNull("error")) {
                result.message = node.get("error").asText();
            } else if (node.hasNonNull("message")) {
                result.message = node.get("message").asText();
            } else if (node.hasNonNull("detail")) {
                result.message = node.get("detail").asText();
            }
            if (node.hasNonNull("code")) {
                result.code = node.get("code").asText();
            }
            if (node.hasNonNull("hint")) {
                result.hint = node.get("hint").asText();
            }
            if (node.has("errors") && node.get("errors").isArray()) {
                List<String> errors = new ArrayList<>();
                for (JsonNode e : node.get("errors")) {
                    errors.add(e.isTextual() ? e.asText() : e.toString());
                }
                result.errors = errors;
                if (result.message == null || result.message.isBlank() || result.message.equals(truncate(trimmed, 500))) {
                    result.message = String.join("; ", errors);
                }
            }
        } catch (Exception ignored) {
            // plain-text or non-JSON body
        }
        if (result.message == null || result.message.isBlank()) {
            result.message = "HTTP error";
        }
        return result;
    }

    private static String truncate(String s, int max) {
        return s.length() <= max ? s : s.substring(0, max);
    }

    private static final class ParsedError {
        String message;
        String code;
        String hint;
        List<String> errors = List.of();
    }
}
