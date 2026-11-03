package com.acugis.reports;

import java.math.BigDecimal;
import java.sql.Connection;
import java.sql.ResultSet;
import java.sql.ResultSetMetaData;
import java.sql.SQLException;
import java.sql.Types;
import java.util.ArrayList;
import java.util.Collection;
import java.util.HashMap;
import java.util.LinkedHashMap;
import java.util.List;
import java.util.Locale;
import java.util.Map;

import com.fasterxml.jackson.databind.ObjectMapper;
import com.fasterxml.jackson.databind.node.ArrayNode;
import com.fasterxml.jackson.databind.node.ObjectNode;

import net.sf.jasperreports.engine.DefaultJasperReportsContext;
import net.sf.jasperreports.engine.JRDataset;
import net.sf.jasperreports.engine.JRException;
import net.sf.jasperreports.engine.JRExpression;
import net.sf.jasperreports.engine.JRParameter;
import net.sf.jasperreports.engine.JRPropertiesHolder;
import net.sf.jasperreports.engine.JRPropertiesMap;
import net.sf.jasperreports.engine.JRQuery;
import net.sf.jasperreports.engine.JRResultSetDataSource;
import net.sf.jasperreports.engine.JRValueParameter;
import net.sf.jasperreports.engine.JasperReport;
import net.sf.jasperreports.engine.JasperReportsContext;
import net.sf.jasperreports.engine.query.JRQueryExecuter;
import net.sf.jasperreports.engine.query.QueryExecuterFactory;
import net.sf.jasperreports.engine.type.ParameterEvaluationTimeEnum;
import net.sf.jasperreports.engine.util.JRQueryExecuterUtils;

/**
 * Executes the main report dataset query through JasperReports' query executer
 * (so {@code $P{}}, {@code $P!{}}, and {@code $X{}} expand natively) and returns
 * tabular JSON built from the JDBC result set. Subdatasets are ignored.
 */
public final class ReportQueryRunner {

    private static final ObjectMapper MAPPER = new ObjectMapper();
    private static final int DEFAULT_MAX_ROWS = 10000;

    private ReportQueryRunner() {
    }

    public static ObjectNode execute(
            JasperReport report,
            Map<String, Object> userParams,
            Connection connection,
            Integer maxRows) throws JRException, SQLException {
        if (report == null) {
            throw new JRException("Report is required");
        }
        if (connection == null) {
            throw new JRException("Connection is required");
        }

        JRDataset dataset = report.getMainDataset();
        if (dataset == null) {
            throw new JRException("Report has no main dataset");
        }
        JRQuery query = dataset.getQuery();
        if (query == null || query.getText() == null || query.getText().trim().isEmpty()) {
            throw new JRException("Report main dataset has no query");
        }

        int limit = maxRows == null || maxRows <= 0 ? DEFAULT_MAX_ROWS : maxRows;
        JasperReportsContext context = DefaultJasperReportsContext.getInstance();

        Map<String, Object> values = new HashMap<>();
        if (userParams != null) {
            values.putAll(userParams);
        }
        values.put(JRParameter.REPORT_CONNECTION, connection);
        values.put(JRParameter.REPORT_MAX_COUNT, Integer.valueOf(limit));
        values.put(JRParameter.REPORT_PARAMETERS_MAP, values);
        values.put(JRParameter.JASPER_REPORTS_CONTEXT, context);
        values.put(JRParameter.JASPER_REPORT, report);

        Map<String, JRValueParameter> paramMap = buildValueParameters(report, values);

        String language = query.getLanguage();
        if (language == null || language.trim().isEmpty()) {
            language = "sql";
        }

        QueryExecuterFactory factory = JRQueryExecuterUtils.getInstance(context).getExecuterFactory(language);
        JRQueryExecuter executer = factory.createQueryExecuter(context, dataset, paramMap);
        try {
            Object ds = executer.createDatasource();
            if (!(ds instanceof JRResultSetDataSource)) {
                throw new JRException("Query executer did not return a JDBC result set (language=" + language + ")");
            }
            ResultSet rs = ((JRResultSetDataSource) ds).getResultSet();
            return materialize(rs, limit, language);
        } finally {
            try {
                executer.close();
            } catch (Exception ignored) {
                // best-effort cleanup
            }
        }
    }

    private static ObjectNode materialize(ResultSet rs, int limit, String language) throws SQLException {
        ResultSetMetaData meta = rs.getMetaData();
        int colCount = meta.getColumnCount();

        ArrayNode columns = MAPPER.createArrayNode();
        List<String> names = new ArrayList<>(colCount);
        String geometryColumn = null;

        for (int i = 1; i <= colCount; i++) {
            String label = meta.getColumnLabel(i);
            if (label == null || label.trim().isEmpty()) {
                label = meta.getColumnName(i);
            }
            String name = label == null ? ("col_" + i) : label;
            names.add(name);

            String typeName = meta.getColumnTypeName(i);
            if (typeName == null) {
                typeName = "";
            }
            String javaType = mapJavaType(meta.getColumnType(i), typeName, meta.getColumnClassName(i));
            boolean geometry = isGeometry(typeName, name);
            if (geometry && geometryColumn == null) {
                geometryColumn = name;
            }

            ObjectNode col = MAPPER.createObjectNode();
            col.put("name", name);
            col.put("type", typeName);
            col.put("java_type", javaType);
            col.put("nullable", meta.isNullable(i) != ResultSetMetaData.columnNoNulls);
            col.put("is_geometry", geometry);
            col.put("ordinal", i);
            columns.add(col);
        }

        ArrayNode rows = MAPPER.createArrayNode();
        int rowCount = 0;
        boolean truncated = false;
        while (rs.next()) {
            if (rowCount >= limit) {
                truncated = true;
                break;
            }
            ObjectNode row = MAPPER.createObjectNode();
            for (int i = 1; i <= colCount; i++) {
                boolean geometry = columns.get(i - 1).path("is_geometry").asBoolean(false);
                Object value = readCellValue(rs, i, geometry);
                putJsonValue(row, names.get(i - 1), value);
            }
            rows.add(row);
            rowCount++;
        }

        // Second pass: detect geometry from values when JDBC type/name heuristics missed it.
        if (geometryColumn == null && rowCount > 0) {
            geometryColumn = detectGeometryFromRows(columns, rows);
            if (geometryColumn != null) {
                for (int i = 0; i < columns.size(); i++) {
                    ObjectNode col = (ObjectNode) columns.get(i);
                    if (geometryColumn.equals(col.path("name").asText())) {
                        col.put("is_geometry", true);
                    }
                }
            }
        }

        ObjectNode out = MAPPER.createObjectNode();
        out.put("engine", language);
        out.set("columns", columns);
        out.set("rows", rows);
        out.put("total_rows", rowCount);
        out.put("truncated", truncated);
        if (geometryColumn != null) {
            out.put("geometry_column", geometryColumn);
            out.put("spatial_capable", true);
        } else {
            out.put("spatial_capable", false);
        }
        return out;
    }

    private static Object readCellValue(ResultSet rs, int index, boolean geometry) throws SQLException {
        if (geometry) {
            // Prefer textual forms (WKT / EWKB hex) so Data Explorer can map them.
            String text = rs.getString(index);
            if (text != null) {
                return text;
            }
            if (rs.wasNull()) {
                return null;
            }
        }
        Object value = rs.getObject(index);
        if (value == null) {
            return null;
        }
        String className = value.getClass().getName();
        if ("org.postgresql.util.PGobject".equals(className)) {
            try {
                Object pgType = value.getClass().getMethod("getType").invoke(value);
                Object pgValue = value.getClass().getMethod("getValue").invoke(value);
                String type = pgType == null ? "" : String.valueOf(pgType).toLowerCase(Locale.ROOT);
                if (type.contains("geometry") || type.contains("geography") || geometry) {
                    return pgValue == null ? null : String.valueOf(pgValue);
                }
            } catch (ReflectiveOperationException ignored) {
                // fall through
            }
        }
        return value;
    }

    private static String detectGeometryFromRows(ArrayNode columns, ArrayNode rows) {
        int sample = Math.min(rows.size(), 25);
        for (int c = 0; c < columns.size(); c++) {
            ObjectNode col = (ObjectNode) columns.get(c);
            String name = col.path("name").asText("");
            if (isGeometry(col.path("type").asText(""), name)) {
                return name;
            }
            int hits = 0;
            for (int r = 0; r < sample; r++) {
                String v = rows.get(r).path(name).asText(null);
                if (looksLikeGeometryText(v)) {
                    hits++;
                }
            }
            if (hits > 0) {
                return name;
            }
        }
        return null;
    }

    private static boolean looksLikeGeometryText(String value) {
        if (value == null) {
            return false;
        }
        String s = value.trim();
        if (s.isEmpty()) {
            return false;
        }
        String upper = s.toUpperCase(Locale.ROOT);
        if (upper.startsWith("POINT") || upper.startsWith("LINESTRING") || upper.startsWith("POLYGON")
                || upper.startsWith("MULTI") || upper.startsWith("GEOMETRYCOLLECTION")
                || upper.startsWith("SRID=")) {
            return true;
        }
        if (s.startsWith("{") && s.contains("\"type\"")) {
            return true;
        }
        // PostGIS EWKB hex
        if (s.length() >= 18 && s.length() % 2 == 0 && s.matches("(?i)[0-9a-f]+")
                && (s.startsWith("00") || s.startsWith("01") || s.startsWith("00") || s.toLowerCase(Locale.ROOT).startsWith("0101")
                || s.toLowerCase(Locale.ROOT).startsWith("0102") || s.toLowerCase(Locale.ROOT).startsWith("0103")
                || s.toLowerCase(Locale.ROOT).startsWith("0104") || s.toLowerCase(Locale.ROOT).startsWith("0105")
                || s.toLowerCase(Locale.ROOT).startsWith("0106") || s.toLowerCase(Locale.ROOT).startsWith("0107"))) {
            return true;
        }
        return false;
    }

    private static Map<String, JRValueParameter> buildValueParameters(
            JasperReport report,
            Map<String, Object> values) {
        Map<String, JRValueParameter> out = new LinkedHashMap<>();

        JRParameter[] declared = report.getParameters();
        if (declared != null) {
            for (JRParameter p : declared) {
                if (p == null) {
                    continue;
                }
                String name = p.getName();
                Object value;
                if (values.containsKey(name)) {
                    value = values.get(name);
                } else {
                    // Query path (Data Explorer): missing Collection → empty list so
                    // $X{IN} is match-all. Do not apply JRXML defaultValueExpression
                    // here; the report viewer fill path applies defaults instead.
                    Class<?> vc = p.getValueClass();
                    if (vc != null && Collection.class.isAssignableFrom(vc)) {
                        value = new ArrayList<>();
                    } else {
                        value = null;
                    }
                }
                Class<?> valueClass = p.getValueClass() != null ? p.getValueClass() : Object.class;
                out.put(name, new SimpleValueParameter(name, valueClass, value, p.isSystemDefined()));
            }
        }

        for (Map.Entry<String, Object> e : values.entrySet()) {
            if (out.containsKey(e.getKey())) {
                out.get(e.getKey()).setValue(e.getValue());
                continue;
            }
            Object value = e.getValue();
            Class<?> cls = value != null ? value.getClass() : Object.class;
            out.put(e.getKey(), new SimpleValueParameter(e.getKey(), cls, value, true));
        }
        return out;
    }

    private static String mapJavaType(int sqlType, String typeName, String className) {
        String tn = typeName == null ? "" : typeName.toLowerCase(Locale.ROOT);
        if (isGeometry(typeName, "")) {
            return "org.postgis.Geometry";
        }
        if (className != null && !className.isEmpty() && !className.equals("java.lang.Object")) {
            return className;
        }
        switch (sqlType) {
            case Types.INTEGER:
            case Types.SMALLINT:
            case Types.TINYINT:
                return "java.lang.Integer";
            case Types.BIGINT:
                return "java.lang.Long";
            case Types.FLOAT:
            case Types.REAL:
                return "java.lang.Float";
            case Types.DOUBLE:
            case Types.NUMERIC:
            case Types.DECIMAL:
                return "java.lang.Double";
            case Types.BOOLEAN:
            case Types.BIT:
                return "java.lang.Boolean";
            case Types.DATE:
                return "java.sql.Date";
            case Types.TIMESTAMP:
            case Types.TIMESTAMP_WITH_TIMEZONE:
                return "java.sql.Timestamp";
            case Types.TIME:
            case Types.TIME_WITH_TIMEZONE:
                return "java.sql.Time";
            case Types.BLOB:
            case Types.BINARY:
            case Types.VARBINARY:
            case Types.LONGVARBINARY:
                return "[B";
            default:
                if (tn.contains("int")) {
                    return "java.lang.Long";
                }
                if (tn.contains("float") || tn.contains("double") || tn.contains("numeric") || tn.contains("decimal")) {
                    return "java.lang.Double";
                }
                if (tn.contains("bool")) {
                    return "java.lang.Boolean";
                }
                return "java.lang.String";
        }
    }

    private static boolean isGeometry(String typeName, String columnName) {
        String tn = typeName == null ? "" : typeName.toLowerCase(Locale.ROOT);
        if (tn.contains("geometry") || tn.contains("geography")) {
            return true;
        }
        String cn = columnName == null ? "" : columnName.toLowerCase(Locale.ROOT);
        return cn.equals("geometry") || cn.equals("geom") || cn.equals("shape") || cn.equals("the_geom")
                || cn.equals("wkb_geometry") || cn.equals("geog") || cn.equals("geography")
                || cn.equals("location") || cn.equals("way") || cn.endsWith("_geom") || cn.endsWith("_geometry");
    }

    private static void putJsonValue(ObjectNode row, String name, Object value) {
        if (value == null) {
            row.putNull(name);
            return;
        }
        if (value instanceof Boolean) {
            row.put(name, (Boolean) value);
        } else if (value instanceof Integer) {
            row.put(name, (Integer) value);
        } else if (value instanceof Long) {
            row.put(name, (Long) value);
        } else if (value instanceof Short) {
            row.put(name, ((Short) value).intValue());
        } else if (value instanceof Byte) {
            row.put(name, ((Byte) value).intValue());
        } else if (value instanceof Float) {
            row.put(name, ((Float) value).doubleValue());
        } else if (value instanceof Double) {
            row.put(name, (Double) value);
        } else if (value instanceof BigDecimal) {
            row.put(name, ((BigDecimal) value).doubleValue());
        } else if (value instanceof Number) {
            row.put(name, ((Number) value).doubleValue());
        } else if (value instanceof byte[]) {
            row.put(name, java.util.Base64.getEncoder().encodeToString((byte[]) value));
        } else {
            row.put(name, String.valueOf(value));
        }
    }

    /**
     * Minimal {@link JRValueParameter} for query-executer parameter maps.
     */
    private static final class SimpleValueParameter implements JRValueParameter {
        private final String name;
        private final Class<?> valueClass;
        private final boolean systemDefined;
        private Object value;

        SimpleValueParameter(String name, Class<?> valueClass, Object value, boolean systemDefined) {
            this.name = name;
            this.valueClass = valueClass != null ? valueClass : Object.class;
            this.value = value;
            this.systemDefined = systemDefined;
        }

        @Override
        public Object getValue() {
            return value;
        }

        @Override
        public void setValue(Object value) {
            this.value = value;
        }

        @Override
        public String getName() {
            return name;
        }

        @Override
        public String getDescription() {
            return null;
        }

        @Override
        public void setDescription(String description) {
            // no-op
        }

        @Override
        public Class<?> getValueClass() {
            return valueClass;
        }

        @Override
        public String getValueClassName() {
            return valueClass.getName();
        }

        @Override
        public boolean isSystemDefined() {
            return systemDefined;
        }

        @Override
        public boolean isForPrompting() {
            return !systemDefined;
        }

        @Override
        public ParameterEvaluationTimeEnum getEvaluationTime() {
            return ParameterEvaluationTimeEnum.EARLY;
        }

        @Override
        public JRExpression getDefaultValueExpression() {
            return null;
        }

        @Override
        public Class<?> getNestedType() {
            return null;
        }

        @Override
        public String getNestedTypeName() {
            return null;
        }

        @Override
        public boolean hasProperties() {
            return false;
        }

        @Override
        public JRPropertiesMap getPropertiesMap() {
            return new JRPropertiesMap();
        }

        @Override
        public JRPropertiesHolder getParentProperties() {
            return null;
        }

        @Override
        public Object clone() {
            return new SimpleValueParameter(name, valueClass, value, systemDefined);
        }
    }
}
