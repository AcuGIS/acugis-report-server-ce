package com.acugis.report.internal;

import com.fasterxml.jackson.databind.DeserializationFeature;
import com.fasterxml.jackson.databind.JsonNode;
import com.fasterxml.jackson.databind.ObjectMapper;
import com.fasterxml.jackson.databind.SerializationFeature;
import com.fasterxml.jackson.datatype.jsr310.JavaTimeModule;

import java.io.IOException;

/** Shared Jackson configuration for the SDK. */
public final class JsonSupport {

    private static final ObjectMapper MAPPER = new ObjectMapper()
            .registerModule(new JavaTimeModule())
            .disable(SerializationFeature.WRITE_DATES_AS_TIMESTAMPS)
            .configure(DeserializationFeature.FAIL_ON_UNKNOWN_PROPERTIES, false);

    private JsonSupport() {}

    public static ObjectMapper mapper() {
        return MAPPER;
    }

    public static byte[] writeBytes(Object value) {
        try {
            return MAPPER.writeValueAsBytes(value);
        } catch (IOException e) {
            throw new IllegalArgumentException("Failed to serialize JSON body", e);
        }
    }

    public static <T> T read(byte[] body, Class<T> type) throws IOException {
        if (body == null || body.length == 0) {
            return null;
        }
        return MAPPER.readValue(body, type);
    }

    public static JsonNode readTree(byte[] body) throws IOException {
        if (body == null || body.length == 0) {
            return MAPPER.createObjectNode();
        }
        return MAPPER.readTree(body);
    }
}
