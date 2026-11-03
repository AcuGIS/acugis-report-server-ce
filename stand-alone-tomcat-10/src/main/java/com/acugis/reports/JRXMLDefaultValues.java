package com.acugis.reports;

import java.util.ArrayList;
import java.util.Collection;
import java.util.LinkedHashMap;
import java.util.List;
import java.util.Map;
import java.util.regex.Matcher;
import java.util.regex.Pattern;

import net.sf.jasperreports.engine.JRExpression;
import net.sf.jasperreports.engine.JRParameter;
import net.sf.jasperreports.engine.JasperReport;

/**
 * Parses common JRXML {@code defaultValueExpression} forms so omitted request
 * parameters can still receive their JRXML defaults (e.g. Arrays.asList("Austin")).
 * Complex expressions are left unevaluated — Jasper fill handles those when the
 * parameter is absent from the fill map.
 */
final class JRXMLDefaultValues {

    private static final Pattern QUOTED = Pattern.compile("\"((?:\\\\.|[^\"\\\\])*)\"|'((?:\\\\.|[^'\\\\])*)'");
    private static final Pattern AS_LIST = Pattern.compile(
            "(?i)(?:java\\.util\\.)?(?:Arrays\\.asList|List\\.of|Collections\\.singletonList|Collections\\.singleton)\\s*\\(");
    private static final Pattern EMPTY_LIST = Pattern.compile("(?i)(?:java\\.util\\.)?Collections\\.emptyList\\s*\\(\\s*\\)");
    private static final Pattern INT_VALUE = Pattern.compile("(?i)(?:java\\.lang\\.)?Integer\\.valueOf\\s*\\(\\s*(-?\\d+)\\s*\\)");
    private static final Pattern LONG_VALUE = Pattern.compile("(?i)(?:java\\.lang\\.)?Long\\.valueOf\\s*\\(\\s*(-?\\d+)\\s*\\)");

    private JRXMLDefaultValues() {
    }

    /**
     * For each non-system parameter missing from {@code params}, insert a parsed
     * JRXML default when the expression is a supported literal form.
     */
    static void applyMissing(Map<String, Object> params, JasperReport report) {
        if (params == null || report == null) {
            return;
        }
        JRParameter[] declared = report.getParameters();
        if (declared == null) {
            return;
        }
        for (JRParameter parameter : declared) {
            if (parameter == null || parameter.isSystemDefined()) {
                continue;
            }
            String name = parameter.getName();
            if (name == null || params.containsKey(name)) {
                continue;
            }
            Object value = evaluate(parameter);
            if (value != null) {
                params.put(name, value);
            }
        }
    }

    static Map<String, Object> missingDefaults(JasperReport report, Map<String, Object> existing) {
        Map<String, Object> out = new LinkedHashMap<>();
        if (report == null) {
            return out;
        }
        JRParameter[] declared = report.getParameters();
        if (declared == null) {
            return out;
        }
        for (JRParameter parameter : declared) {
            if (parameter == null || parameter.isSystemDefined()) {
                continue;
            }
            String name = parameter.getName();
            if (name == null || (existing != null && existing.containsKey(name))) {
                continue;
            }
            Object value = evaluate(parameter);
            if (value != null) {
                out.put(name, value);
            }
        }
        return out;
    }

    static Object evaluate(JRParameter parameter) {
        if (parameter == null) {
            return null;
        }
        JRExpression expression = parameter.getDefaultValueExpression();
        if (expression == null) {
            return null;
        }
        String text = expression.getText();
        if (text == null) {
            return null;
        }
        text = text.trim();
        if (text.isEmpty() || "null".equalsIgnoreCase(text)) {
            return null;
        }

        Class<?> valueClass = parameter.getValueClass();
        boolean collection = valueClass != null && Collection.class.isAssignableFrom(valueClass);

        if (EMPTY_LIST.matcher(text).matches()) {
            return new ArrayList<>();
        }
        if (AS_LIST.matcher(text).find()) {
            return extractQuotedStrings(text);
        }

        Matcher intMatch = INT_VALUE.matcher(text);
        if (intMatch.matches()) {
            return Integer.valueOf(intMatch.group(1));
        }
        Matcher longMatch = LONG_VALUE.matcher(text);
        if (longMatch.matches()) {
            return Long.valueOf(longMatch.group(1));
        }

        if ("true".equalsIgnoreCase(text) || "Boolean.TRUE".equalsIgnoreCase(text)) {
            return Boolean.TRUE;
        }
        if ("false".equalsIgnoreCase(text) || "Boolean.FALSE".equalsIgnoreCase(text)) {
            return Boolean.FALSE;
        }

        String unquoted = unquote(text);
        if (unquoted != null) {
            if (collection) {
                List<Object> list = new ArrayList<>(1);
                list.add(unquoted);
                return list;
            }
            return unquoted;
        }

        if (text.matches("-?\\d+")) {
            try {
                return Integer.valueOf(text);
            } catch (NumberFormatException ignored) {
                return text;
            }
        }
        return null;
    }

    private static List<Object> extractQuotedStrings(String expr) {
        List<Object> out = new ArrayList<>();
        Matcher m = QUOTED.matcher(expr);
        while (m.find()) {
            String raw = m.group(1) != null ? m.group(1) : m.group(2);
            out.add(unescape(raw));
        }
        return out;
    }

    private static String unquote(String expr) {
        if (expr == null || expr.length() < 2) {
            return null;
        }
        char q = expr.charAt(0);
        if ((q != '"' && q != '\'') || expr.charAt(expr.length() - 1) != q) {
            return null;
        }
        return unescape(expr.substring(1, expr.length() - 1));
    }

    private static String unescape(String s) {
        if (s == null || s.indexOf('\\') < 0) {
            return s;
        }
        StringBuilder b = new StringBuilder(s.length());
        for (int i = 0; i < s.length(); i++) {
            char c = s.charAt(i);
            if (c != '\\' || i + 1 >= s.length()) {
                b.append(c);
                continue;
            }
            char n = s.charAt(++i);
            switch (n) {
                case 'n':
                    b.append('\n');
                    break;
                case 't':
                    b.append('\t');
                    break;
                case 'r':
                    b.append('\r');
                    break;
                case '"':
                case '\'':
                case '\\':
                    b.append(n);
                    break;
                default:
                    b.append(n);
                    break;
            }
        }
        return b.toString();
    }
}
