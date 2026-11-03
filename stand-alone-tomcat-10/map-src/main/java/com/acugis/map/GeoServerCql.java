package com.acugis.map;

import java.util.ArrayList;
import java.util.Collection;
import java.util.Collections;
import java.util.List;

/**
 * Build GeoServer CQL_FILTER clauses from report parameter values (Publisher-aligned).
 *
 * Multi-value / Collection parameters become {@code IN (...)} even when the mapping
 * operator is {@code =}. Empty / null values omit the clause.
 */
public final class GeoServerCql {

    private GeoServerCql() {}

    /**
     * Combine non-empty clauses with {@code AND}. Null/blank clauses are skipped.
     * Returns null when nothing remains (omits CQL_FILTER).
     */
    public static String and(String... clauses) {
        if (clauses == null || clauses.length == 0) {
            return null;
        }
        StringBuilder b = new StringBuilder();
        for (String c : clauses) {
            if (c == null) {
                continue;
            }
            String t = c.trim();
            if (t.isEmpty()) {
                continue;
            }
            if (b.length() > 0) {
                b.append(" AND ");
            }
            b.append(t);
        }
        return b.length() == 0 ? null : b.toString();
    }

    /**
     * One mapping row → CQL clause, or null when the parameter has no usable values.
     */
    public static String clause(String field, String operator, Object value) {
        String f = field == null ? "" : field.trim();
        if (f.isEmpty() || !f.matches("^[A-Za-z_][A-Za-z0-9_]*$")) {
            return null;
        }
        List<Object> values = filterValues(value);
        if (values.isEmpty()) {
            return null;
        }

        String op = operator == null || operator.trim().isEmpty() ? "=" : operator.trim();
        String opKey = op.toUpperCase();
        if ("LIKE".equals(opKey) || "ILIKE".equals(opKey) || "IN".equals(opKey)) {
            op = opKey;
        }

        boolean multi = values.size() > 1;
        // Multi-select report params naturally become IN (...).
        if (multi && !"LIKE".equals(op) && !"ILIKE".equals(op)) {
            op = "IN";
        }

        if ("IN".equals(op) || (multi && !"LIKE".equals(op) && !"ILIKE".equals(op))) {
            StringBuilder b = new StringBuilder();
            b.append(f).append(" IN (");
            for (int i = 0; i < values.size(); i++) {
                if (i > 0) {
                    b.append(',');
                }
                b.append(literal(values.get(i)));
            }
            b.append(')');
            return b.toString();
        }

        if ("LIKE".equals(op) || "ILIKE".equals(op)) {
            if (!multi) {
                return f + " " + op + " " + literal(values.get(0));
            }
            StringBuilder b = new StringBuilder("(");
            for (int i = 0; i < values.size(); i++) {
                if (i > 0) {
                    b.append(" OR ");
                }
                b.append(f).append(' ').append(op).append(' ').append(literal(values.get(i)));
            }
            b.append(')');
            return b.toString();
        }

        if (!("=".equals(op) || "<>".equals(op) || ">".equals(op) || ">=".equals(op)
                || "<".equals(op) || "<=".equals(op))) {
            op = "=";
        }
        return f + op + literal(values.get(0));
    }

    @SuppressWarnings("unchecked")
    private static List<Object> filterValues(Object raw) {
        if (raw == null) {
            return Collections.emptyList();
        }
        List<Object> out = new ArrayList<Object>();
        if (raw instanceof Collection) {
            for (Object item : (Collection<?>) raw) {
                addScalar(out, item);
            }
            return out;
        }
        if (raw.getClass().isArray()) {
            Object[] arr = (Object[]) raw;
            for (Object item : arr) {
                addScalar(out, item);
            }
            return out;
        }
        addScalar(out, raw);
        return out;
    }

    private static void addScalar(List<Object> out, Object item) {
        if (item == null || item instanceof Collection) {
            return;
        }
        if (item instanceof Boolean || item instanceof Number) {
            out.add(item);
            return;
        }
        String s = String.valueOf(item).trim();
        if (!s.isEmpty()) {
            out.add(s);
        }
    }

    private static String literal(Object value) {
        if (value instanceof Boolean) {
            return ((Boolean) value).booleanValue() ? "true" : "false";
        }
        if (value instanceof Number) {
            return String.valueOf(value);
        }
        String s = String.valueOf(value);
        // Numeric strings unquoted (Publisher-aligned); leading-zero codes stay quoted.
        if (s.matches("-?\\d+(\\.\\d+)?([eE][+-]?\\d+)?") && !s.matches("0\\d+")) {
            return s;
        }
        return "'" + s.replace("'", "''") + "'";
    }
}
