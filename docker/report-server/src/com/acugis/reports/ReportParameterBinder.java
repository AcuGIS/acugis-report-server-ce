package com.acugis.reports;

import java.io.File;
import java.math.BigDecimal;
import java.math.BigInteger;
import java.util.ArrayList;
import java.util.Collection;
import java.util.HashMap;
import java.util.HashSet;
import java.util.List;
import java.util.Map;
import java.util.Set;
import java.util.logging.Level;
import java.util.logging.Logger;

import javax.servlet.http.HttpServletRequest;
import net.sf.jasperreports.engine.JRException;
import net.sf.jasperreports.engine.JRParameter;
import net.sf.jasperreports.engine.JasperCompileManager;
import net.sf.jasperreports.engine.JasperReport;
import net.sf.jasperreports.engine.util.JRLoader;

/**
 * Binds HTTP request parameters to JasperReports parameter values using report metadata.
 * HTTP values arrive as strings; this binder coerces them to the JRXML-declared types
 * (e.g. p_srid java.lang.Integer) so fillReport does not ClassCastException.
 */
public final class ReportParameterBinder {

    private static final Logger LOG = Logger.getLogger(ReportParameterBinder.class.getName());
    private static final String[] RESERVED_PARAMS = {"SUBREPORT_DIR", "REPORT_DIR"};
    /** Viewer sentinel: explicit empty Collection that must override JRXML defaults. */
    static final String CLEAR_SENTINEL = "__JRS_CLEAR__";

    private ReportParameterBinder() {
    }

    public static JasperReport loadOrCompileReport(File reportFile) throws JRException {
        String name = reportFile.getName().toLowerCase();
        if (name.endsWith(".jrxml")) {
            return JasperCompileManager.compileReport(reportFile.getAbsolutePath());
        }
        return (JasperReport) JRLoader.loadObject(reportFile);
    }

    public static Set<String> collectionParameterNames(JasperReport report) {
        Set<String> names = new HashSet<>();
        if (report == null) {
            return names;
        }
        JRParameter[] parameters = report.getParameters();
        if (parameters == null) {
            return names;
        }
        for (JRParameter parameter : parameters) {
            if (parameter == null || parameter.isSystemDefined()) {
                continue;
            }
            Class<?> valueClass = parameter.getValueClass();
            if (valueClass != null && Collection.class.isAssignableFrom(valueClass)) {
                names.add(parameter.getName());
            }
        }
        return names;
    }

    /**
     * Map of non-system JRXML parameter name → value class.
     */
    public static Map<String, Class<?>> parameterValueClasses(JasperReport report) {
        Map<String, Class<?>> types = new HashMap<>();
        if (report == null) {
            return types;
        }
        JRParameter[] parameters = report.getParameters();
        if (parameters == null) {
            return types;
        }
        for (JRParameter parameter : parameters) {
            if (parameter == null || parameter.isSystemDefined()) {
                continue;
            }
            Class<?> valueClass = parameter.getValueClass();
            if (valueClass != null) {
                types.put(parameter.getName(), valueClass);
            }
        }
        return types;
    }

    /**
     * Preferred entry point: coerce HTTP string values using JRXML parameter types.
     * Applies JRXML defaults for omitted parameters (report fill / viewer).
     */
    public static Map<String, Object> bindParameters(HttpServletRequest request, JasperReport report) {
        return bindParameters(request, report, true);
    }

    /**
     * @param applyJrxmlDefaults false for Data Explorer report-query (match-all / no baked defaults).
     */
    public static Map<String, Object> bindParameters(
            HttpServletRequest request,
            JasperReport report,
            boolean applyJrxmlDefaults) {
        Map<String, Object> params = bindParameters(request, collectionParameterNames(report), parameterValueClasses(report));
        if (applyJrxmlDefaults) {
            JRXMLDefaultValues.applyMissing(params, report);
        }
        return params;
    }

    /**
     * Backward-compatible overload (no scalar coercion). Prefer
     * {@link #bindParameters(HttpServletRequest, JasperReport)}.
     */
    public static Map<String, Object> bindParameters(
            HttpServletRequest request,
            Set<String> collectionParamNames) {
        return bindParameters(request, collectionParamNames, null);
    }

    public static Map<String, Object> bindParameters(
            HttpServletRequest request,
            Set<String> collectionParamNames,
            Map<String, Class<?>> valueClasses) {
        Map<String, Object> params = new HashMap<>();
        Map<String, String[]> parameterMap = request.getParameterMap();

        for (Map.Entry<String, String[]> entry : parameterMap.entrySet()) {
            String key = entry.getKey();
            if (key.startsWith("_")) {
                continue;
            }
            if (isReserved(key)) {
                continue;
            }

            String[] values = entry.getValue();
            if (values == null || values.length == 0) {
                continue;
            }

            Class<?> valueClass = valueClasses != null ? valueClasses.get(key) : null;
            boolean isCollection = (collectionParamNames != null && collectionParamNames.contains(key))
                    || (valueClass != null && Collection.class.isAssignableFrom(valueClass));

            if (isCollection || values.length > 1) {
                List<Object> list = coerceCollection(values, valueClass);
                if (list == null) {
                    continue;
                }
                params.put(key, list);
            } else {
                Object coerced = coerceValue(values[0], valueClass);
                if (coerced != null) {
                    params.put(key, coerced);
                }
            }
        }
        return params;
    }

    /**
     * Coerce a single HTTP string to the JRXML-declared Java type.
     * Returns the original string when the type is unknown or conversion fails.
     */
    static Object coerceValue(String raw, Class<?> valueClass) {
        if (raw == null) {
            return null;
        }
        if (valueClass == null
                || valueClass == String.class
                || valueClass == Object.class
                || Collection.class.isAssignableFrom(valueClass)) {
            return raw;
        }

        String s = raw.trim();
        if (s.isEmpty()) {
            return null;
        }

        try {
            if (valueClass == Integer.class || valueClass == int.class) {
                return Integer.valueOf(s);
            }
            if (valueClass == Long.class || valueClass == long.class) {
                return Long.valueOf(s);
            }
            if (valueClass == Short.class || valueClass == short.class) {
                return Short.valueOf(s);
            }
            if (valueClass == Byte.class || valueClass == byte.class) {
                return Byte.valueOf(s);
            }
            if (valueClass == Double.class || valueClass == double.class) {
                return Double.valueOf(s);
            }
            if (valueClass == Float.class || valueClass == float.class) {
                return Float.valueOf(s);
            }
            if (valueClass == BigDecimal.class) {
                return new BigDecimal(s);
            }
            if (valueClass == BigInteger.class) {
                return new BigInteger(s);
            }
            if (valueClass == Boolean.class || valueClass == boolean.class) {
                if ("1".equals(s) || "true".equalsIgnoreCase(s) || "yes".equalsIgnoreCase(s)) {
                    return Boolean.TRUE;
                }
                if ("0".equals(s) || "false".equalsIgnoreCase(s) || "no".equalsIgnoreCase(s)) {
                    return Boolean.FALSE;
                }
                return Boolean.valueOf(s);
            }
        } catch (NumberFormatException ex) {
            LOG.log(Level.WARNING,
                    "Could not coerce parameter value \"" + raw + "\" to " + valueClass.getName()
                            + "; passing as String",
                    ex);
            return raw;
        }
        return raw;
    }

    private static List<Object> coerceCollection(String[] values, Class<?> valueClass) {
        List<Object> list = new ArrayList<>();
        if (values == null) {
            return null;
        }
        boolean sawClear = false;
        for (String raw : values) {
            if (raw == null) {
                continue;
            }
            String trimmed = raw.trim();
            if (trimmed.isEmpty()) {
                continue;
            }
            if (CLEAR_SENTINEL.equals(trimmed)) {
                sawClear = true;
                continue;
            }
            list.add(trimmed);
        }
        if (!list.isEmpty()) {
            return list;
        }
        if (sawClear) {
            return list;
        }
        return null;
    }

    private static boolean isReserved(String key) {
        for (String reserved : RESERVED_PARAMS) {
            if (key.equals(reserved)) {
                return true;
            }
        }
        return false;
    }
}
