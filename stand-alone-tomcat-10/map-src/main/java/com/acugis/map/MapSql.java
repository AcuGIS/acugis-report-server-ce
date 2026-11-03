package com.acugis.map;

import java.util.Collection;
import java.util.Collections;

/**
 * SQL fragment helpers used from JRXML image expressions when expanding
 * Jasper {@code $X{IN,...}} / {@code $X{NOTIN,...}} into a fully resolved
 * SQL string for {@link MapRenderer}.
 */
public final class MapSql {

    private MapSql() {}

    /**
     * Expand {@code $X{IN, column, param}} into {@code column IN (...)} or
     * {@code 0 = 0} when the collection is null/empty (JasperReports-compatible:
     * empty IN is optional / match-all, so cascade children without a selection
     * still show features for the selected parents).
     */
    public static String in(String column, Object values) {
        return clause(column, values, true);
    }

    /**
     * Expand {@code $X{NOTIN, column, param}} into {@code column NOT IN (...)} or
     * {@code 0 = 0} when empty.
     */
    public static String notIn(String column, Object values) {
        return clause(column, values, false);
    }

    /**
     * Equality predicate for Current Group map extent:
     * {@code column = <literal>} or {@code column IS NULL} when value is null.
     */
    public static String eq(String column, Object value) {
        String col = column == null ? "" : column.trim();
        if (col.isEmpty()) {
            return "1 = 1";
        }
        if (value == null) {
            return col + " IS NULL";
        }
        return col + " = " + literal(value);
    }

    /** SQL literal for a scalar (quoted string / number / boolean / NULL). */
    public static String literal(Object value) {
        if (value == null) {
            return "NULL";
        }
        if (value instanceof Number || value instanceof Boolean) {
            return String.valueOf(value);
        }
        String s = String.valueOf(value);
        return "'" + s.replace("'", "''") + "'";
    }

    private static String clause(String column, Object values, boolean in) {
        Collection<?> coll = asCollection(values);
        if (coll == null || coll.isEmpty()) {
            // JasperReports empty-collection behaviour for IN and NOTIN is match-all.
            return "0 = 0";
        }
        StringBuilder b = new StringBuilder();
        b.append(column == null ? "" : column.trim());
        b.append(in ? " IN (" : " NOT IN (");
        boolean first = true;
        for (Object v : coll) {
            if (!first) {
                b.append(", ");
            }
            first = false;
            b.append(literal(v));
        }
        b.append(')');
        return b.toString();
    }

    @SuppressWarnings("unchecked")
    private static Collection<?> asCollection(Object values) {
        if (values == null) {
            return null;
        }
        if (values instanceof String) {
            String s = ((String) values).trim();
            if (s.isEmpty()) {
                return null;
            }
            return Collections.singletonList(s);
        }
        if (values instanceof Collection) {
            Collection<?> raw = (Collection<?>) values;
            java.util.ArrayList<Object> cleaned = new java.util.ArrayList<Object>();
            for (Object item : raw) {
                if (item == null) {
                    continue;
                }
                if (item instanceof String && ((String) item).trim().isEmpty()) {
                    continue;
                }
                cleaned.add(item);
            }
            return cleaned.isEmpty() ? null : cleaned;
        }
        if (values.getClass().isArray()) {
            Object[] arr = (Object[]) values;
            java.util.List<Object> list = new java.util.ArrayList<Object>(arr.length);
            Collections.addAll(list, arr);
            return asCollection(list);
        }
        return Collections.singletonList(values);
    }

}
