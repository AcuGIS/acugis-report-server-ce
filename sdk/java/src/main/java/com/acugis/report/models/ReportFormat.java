package com.acugis.report.models;

import com.acugis.report.errors.ValidationException;

import java.util.Arrays;
import java.util.Locale;
import java.util.Set;
import java.util.stream.Collectors;

/** Supported {@code _repFormat} values for {@code /reports/execute}. */
public enum ReportFormat {
    HTML,
    PDF,
    XLSX,
    XLS,
    CSV,
    DOCX,
    RTF,
    ODT,
    ODS,
    PPTX,
    XML;

    private static final Set<String> NAMES =
            Arrays.stream(values()).map(f -> f.name().toLowerCase(Locale.ROOT)).collect(Collectors.toSet());

    public String wireValue() {
        return name().toLowerCase(Locale.ROOT);
    }

    public static ReportFormat parse(String format) {
        if (format == null || format.isBlank()) {
            return PDF;
        }
        String normalized = format.trim().toLowerCase(Locale.ROOT);
        if (normalized.startsWith(".")) {
            normalized = normalized.substring(1);
        }
        if (!NAMES.contains(normalized)) {
            throw new ValidationException(
                    "Unsupported format '" + format + "'. Supported: " + String.join(", ", NAMES));
        }
        return ReportFormat.valueOf(normalized.toUpperCase(Locale.ROOT));
    }

    public static Set<String> supported() {
        return Set.copyOf(NAMES);
    }
}
