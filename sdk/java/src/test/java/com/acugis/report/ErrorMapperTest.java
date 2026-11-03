package com.acugis.report;

import com.acugis.report.errors.ConflictException;
import com.acugis.report.errors.ErrorMapper;
import com.acugis.report.errors.NotFoundException;
import com.acugis.report.errors.PermissionDeniedException;
import com.acugis.report.errors.ServerException;
import com.acugis.report.errors.ValidationException;
import com.acugis.report.internal.JsonSupport;
import org.junit.jupiter.api.Test;

import java.nio.charset.StandardCharsets;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertInstanceOf;
import static org.junit.jupiter.api.Assertions.assertTrue;

class ErrorMapperTest {

    @Test
    void mapsStatusCodes() {
        assertInstanceOf(
                ValidationException.class,
                ErrorMapper.fromStatus(400, json("bad request"), JsonSupport.mapper()));
        assertInstanceOf(
                PermissionDeniedException.class,
                ErrorMapper.fromStatus(403, json("Permission denied"), JsonSupport.mapper()));
        assertInstanceOf(
                NotFoundException.class,
                ErrorMapper.fromStatus(404, json("not found"), JsonSupport.mapper()));
        assertInstanceOf(
                ConflictException.class,
                ErrorMapper.fromStatus(409, json("conflict"), JsonSupport.mapper()));
        assertInstanceOf(
                ServerException.class,
                ErrorMapper.fromStatus(503, json("unavailable"), JsonSupport.mapper()));
    }

    @Test
    void parsesValidationErrorsArray() {
        byte[] body =
                "{\"error\":\"validation failed\",\"errors\":[\"name is required\"]}"
                        .getBytes(StandardCharsets.UTF_8);
        ValidationException ex =
                (ValidationException) ErrorMapper.fromStatus(422, body, JsonSupport.mapper());
        assertEquals("validation failed", ex.getMessage());
        assertTrue(ex.getErrors().contains("name is required"));
    }

    @Test
    void plainTextErrors() {
        byte[] body = "Jasper gateway unavailable".getBytes(StandardCharsets.UTF_8);
        ServerException ex = (ServerException) ErrorMapper.fromStatus(503, body, JsonSupport.mapper());
        assertEquals("Jasper gateway unavailable", ex.getMessage());
    }

    private static byte[] json(String error) {
        return ("{\"error\":\"" + error + "\"}").getBytes(StandardCharsets.UTF_8);
    }
}
