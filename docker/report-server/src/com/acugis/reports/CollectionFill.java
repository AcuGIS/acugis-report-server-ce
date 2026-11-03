package com.acugis.reports;

import com.fasterxml.jackson.databind.JsonNode;
import com.fasterxml.jackson.databind.ObjectMapper;
import java.io.IOException;
import java.math.BigDecimal;
import java.math.BigInteger;
import java.sql.Timestamp;
import java.text.ParseException;
import java.text.SimpleDateFormat;
import java.time.Instant;
import java.time.LocalDate;
import java.time.LocalDateTime;
import java.time.ZoneOffset;
import java.time.format.DateTimeFormatter;
import java.time.format.DateTimeParseException;
import java.util.ArrayList;
import java.util.Base64;
import java.util.Date;
import java.util.HashMap;
import java.util.Iterator;
import java.util.List;
import java.util.Locale;
import java.util.Map;
import java.util.UUID;
import net.sf.jasperreports.engine.JRField;
import net.sf.jasperreports.engine.JasperReport;
import net.sf.jasperreports.engine.data.JRMapCollectionDataSource;

/**
 * Parses a JSON array of row objects into a {@link JRMapCollectionDataSource},
 * coercing cell values to JasperReport field classes (BigDecimal, Timestamp, etc.).
 */
public final class CollectionFill {

    private static final ObjectMapper MAPPER = new ObjectMapper();

    private CollectionFill() {
    }

    public static final class ParseResult {
        private final JRMapCollectionDataSource dataSource;
        private final int rowCount;

        ParseResult(JRMapCollectionDataSource dataSource, int rowCount) {
            this.dataSource = dataSource;
            this.rowCount = rowCount;
        }

        public JRMapCollectionDataSource getDataSource() {
            return dataSource;
        }

        public int getRowCount() {
            return rowCount;
        }
    }

    /**
     * @param jsonArray JSON array string, e.g. {@code [{"col":1},{"col":2}]}
     */
    public static ParseResult parse(String jsonArray, JasperReport report) throws IOException {
        if (jsonArray == null || jsonArray.trim().isEmpty()) {
            jsonArray = "[]";
        }
        JsonNode root = MAPPER.readTree(jsonArray);
        if (!root.isArray()) {
            throw new IllegalArgumentException("_collectionData must be a JSON array of row objects");
        }
        Map<String, Class<?>> fieldClasses = fieldValueClasses(report);
        List<Map<String, ?>> rows = new ArrayList<>();
        for (JsonNode rowNode : root) {
            Map<String, Object> row = new HashMap<>();
            if (rowNode != null && rowNode.isObject()) {
                Iterator<Map.Entry<String, JsonNode>> fields = rowNode.fields();
                while (fields.hasNext()) {
                    Map.Entry<String, JsonNode> e = fields.next();
                    String name = e.getKey();
                    JsonNode v = e.getValue();
                    Class<?> target = fieldClasses.get(name);
                    Object coerced = coerceCollectionValue(name, target, v);
                    row.put(name, coerced);
                }
            }
            rows.add(row);
        }
        return new ParseResult(new JRMapCollectionDataSource(rows), rows.size());
    }

    private static Map<String, Class<?>> fieldValueClasses(JasperReport report) {
        Map<String, Class<?>> out = new HashMap<>();
        if (report == null) {
            return out;
        }
        JRField[] fields = report.getFields();
        if (fields == null) {
            return out;
        }
        for (JRField f : fields) {
            if (f == null || f.getName() == null) {
                continue;
            }
            out.put(f.getName(), f.getValueClass());
        }
        return out;
    }

    private static Object coerceCollectionValue(String fieldName, Class<?> target, JsonNode v) {
        if (v == null || v.isNull() || v.isMissingNode()) {
            return null;
        }
        if (target == null || target == Object.class) {
            return defaultJsonValue(v);
        }
        try {
            Object out;
            if (target == BigDecimal.class) {
                out = toBigDecimal(v);
            } else if (target == BigInteger.class) {
                out = toBigInteger(v);
            } else if (target == Byte.class || target == byte.class) {
                out = Byte.valueOf((byte) toLong(v));
            } else if (target == Short.class || target == short.class) {
                out = Short.valueOf((short) toLong(v));
            } else if (target == Integer.class || target == int.class) {
                out = Integer.valueOf((int) toLong(v));
            } else if (target == Long.class || target == long.class) {
                out = Long.valueOf(toLong(v));
            } else if (target == Float.class || target == float.class) {
                out = Float.valueOf((float) toDouble(v));
            } else if (target == Double.class || target == double.class) {
                out = Double.valueOf(toDouble(v));
            } else if (target == Boolean.class || target == boolean.class) {
                out = Boolean.valueOf(toBoolean(v));
            } else if (target == UUID.class) {
                out = UUID.fromString(v.asText().trim());
            } else if (target == Timestamp.class) {
                out = toTimestamp(v);
            } else if (target == java.sql.Date.class) {
                LocalDate d = toLocalDate(v);
                out = java.sql.Date.valueOf(d);
            } else if (target == Date.class) {
                out = new Date(toTimestamp(v).getTime());
            } else if (target == LocalDate.class) {
                out = toLocalDate(v);
            } else if (target == LocalDateTime.class) {
                out = toTimestamp(v).toLocalDateTime();
            } else if (target == byte[].class) {
                out = Base64.getDecoder().decode(v.asText());
            } else if (target == String.class) {
                out = v.isValueNode() ? v.asText() : v.toString();
            } else {
                out = defaultJsonValue(v);
            }
            if (collectionDebug()) {
                System.err.println(
                        "[acugis-collection] field="
                                + fieldName
                                + " declared="
                                + target.getName()
                                + " actual="
                                + (out == null ? "null" : out.getClass().getName()));
            }
            return out;
        } catch (RuntimeException ex) {
            throw new IllegalArgumentException(
                    "Cannot coerce field '"
                            + fieldName
                            + "' value to "
                            + target.getName()
                            + ": "
                            + ex.getMessage(),
                    ex);
        }
    }

    private static boolean collectionDebug() {
        String v = System.getenv("ACUGIS_CASSANDRA_DEBUG");
        if (v != null) {
            v = v.trim().toLowerCase(Locale.ROOT);
            if ("1".equals(v) || "true".equals(v) || "yes".equals(v)) {
                return true;
            }
        }
        v = System.getenv("ACUGIS_DEBUG");
        if (v == null) {
            return false;
        }
        v = v.trim().toLowerCase(Locale.ROOT);
        return "1".equals(v) || "true".equals(v) || "yes".equals(v);
    }

    private static Object defaultJsonValue(JsonNode v) {
        if (v.isNumber()) {
            if (v.isIntegralNumber()) {
                return Long.valueOf(v.asLong());
            }
            return Double.valueOf(v.asDouble());
        }
        if (v.isBoolean()) {
            return Boolean.valueOf(v.asBoolean());
        }
        if (v.isTextual()) {
            return v.asText();
        }
        return v.toString();
    }

    private static BigDecimal toBigDecimal(JsonNode v) {
        if (v.isNumber()) {
            return v.decimalValue();
        }
        String s = v.asText().trim();
        if (s.isEmpty()) {
            return null;
        }
        return new BigDecimal(s);
    }

    private static BigInteger toBigInteger(JsonNode v) {
        if (v.isIntegralNumber()) {
            return v.bigIntegerValue();
        }
        String s = v.asText().trim();
        if (s.isEmpty()) {
            return null;
        }
        return new BigInteger(s);
    }

    private static long toLong(JsonNode v) {
        if (v.isNumber()) {
            return v.asLong();
        }
        String s = v.asText().trim();
        if (s.contains(".") || s.contains("e") || s.contains("E")) {
            return new BigDecimal(s).longValue();
        }
        return Long.parseLong(s);
    }

    private static double toDouble(JsonNode v) {
        if (v.isNumber()) {
            return v.asDouble();
        }
        return Double.parseDouble(v.asText().trim());
    }

    private static boolean toBoolean(JsonNode v) {
        if (v.isBoolean()) {
            return v.asBoolean();
        }
        if (v.isNumber()) {
            return v.asInt() != 0;
        }
        String s = v.asText().trim().toLowerCase(Locale.ROOT);
        if ("true".equals(s) || "1".equals(s) || "yes".equals(s) || "y".equals(s) || "t".equals(s)) {
            return true;
        }
        if ("false".equals(s) || "0".equals(s) || "no".equals(s) || "n".equals(s) || "f".equals(s)) {
            return false;
        }
        throw new IllegalArgumentException("not a boolean: " + s);
    }

    private static Timestamp toTimestamp(JsonNode v) {
        if (v.isNumber()) {
            long n = v.asLong();
            if (n > 100_000_000_000L) {
                return new Timestamp(n);
            }
            return new Timestamp(n * 1000L);
        }
        String s = v.asText().trim();
        if (s.isEmpty()) {
            return null;
        }
        try {
            return Timestamp.from(Instant.parse(s));
        } catch (DateTimeParseException ignore) {
            // continue
        }
        try {
            LocalDateTime ldt = LocalDateTime.parse(s, DateTimeFormatter.ISO_LOCAL_DATE_TIME);
            return Timestamp.valueOf(ldt);
        } catch (DateTimeParseException ignore) {
            // continue
        }
        try {
            LocalDate d = LocalDate.parse(s.substring(0, Math.min(10, s.length())));
            return Timestamp.from(d.atStartOfDay().toInstant(ZoneOffset.UTC));
        } catch (RuntimeException ignore) {
            // continue
        }
        String[] patterns =
                new String[] {
                    "yyyy-MM-dd HH:mm:ss.SSS",
                    "yyyy-MM-dd HH:mm:ss",
                    "yyyy-MM-dd'T'HH:mm:ss.SSSX",
                    "yyyy-MM-dd'T'HH:mm:ssX",
                    "yyyy-MM-dd"
                };
        for (String p : patterns) {
            try {
                Date d = new SimpleDateFormat(p, Locale.US).parse(s);
                return new Timestamp(d.getTime());
            } catch (ParseException ignore) {
                // try next
            }
        }
        throw new IllegalArgumentException("unrecognized timestamp: " + s);
    }

    private static LocalDate toLocalDate(JsonNode v) {
        if (v.isNumber()) {
            return Instant.ofEpochMilli(v.asLong()).atZone(ZoneOffset.UTC).toLocalDate();
        }
        String s = v.asText().trim();
        if (s.length() >= 10 && s.charAt(4) == '-' && s.charAt(7) == '-') {
            return LocalDate.parse(s.substring(0, 10));
        }
        return LocalDate.parse(s);
    }
}
