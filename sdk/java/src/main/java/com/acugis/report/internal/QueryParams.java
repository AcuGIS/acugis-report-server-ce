package com.acugis.report.internal;

import java.net.URLEncoder;
import java.nio.charset.StandardCharsets;
import java.util.ArrayList;
import java.util.Collection;
import java.util.List;
import java.util.Map;
import java.util.Objects;

/** Builds application/x-www-form-urlencoded query / form strings with multi-value support. */
public final class QueryParams {

    private final List<Map.Entry<String, String>> pairs = new ArrayList<>();

    public QueryParams add(String name, Object value) {
        if (value == null) {
            return this;
        }
        if (value instanceof Collection<?> collection) {
            for (Object item : collection) {
                add(name, item);
            }
            return this;
        }
        if (value.getClass().isArray()) {
            int len = java.lang.reflect.Array.getLength(value);
            for (int i = 0; i < len; i++) {
                add(name, java.lang.reflect.Array.get(value, i));
            }
            return this;
        }
        pairs.add(Map.entry(name, String.valueOf(value)));
        return this;
    }

    public QueryParams addAll(Map<String, ?> params) {
        if (params == null) {
            return this;
        }
        for (Map.Entry<String, ?> e : params.entrySet()) {
            add(e.getKey(), e.getValue());
        }
        return this;
    }

    public boolean isEmpty() {
        return pairs.isEmpty();
    }

    public String encode() {
        StringBuilder sb = new StringBuilder();
        for (Map.Entry<String, String> pair : pairs) {
            if (sb.length() > 0) {
                sb.append('&');
            }
            sb.append(enc(pair.getKey())).append('=').append(enc(pair.getValue()));
        }
        return sb.toString();
    }

    public List<Map.Entry<String, String>> entries() {
        return List.copyOf(pairs);
    }

    private static String enc(String s) {
        return URLEncoder.encode(Objects.requireNonNullElse(s, ""), StandardCharsets.UTF_8);
    }
}
