package com.acugis.map;

import java.util.Collections;
import java.util.LinkedHashMap;
import java.util.Map;
import org.locationtech.jts.geom.Geometry;

/** One map feature: geometry plus attribute values for thematic styling. */
final class MapFeature {
    final Geometry geometry;
    final Map<String, Object> attributes;

    MapFeature(Geometry geometry, Map<String, Object> attributes) {
        this.geometry = geometry;
        this.attributes = attributes == null
                ? Collections.<String, Object>emptyMap()
                : Collections.unmodifiableMap(new LinkedHashMap<String, Object>(attributes));
    }

    Object attr(String name) {
        if (name == null || name.isEmpty()) {
            return null;
        }
        if (attributes.containsKey(name)) {
            return attributes.get(name);
        }
        // Case-insensitive fallback.
        for (Map.Entry<String, Object> e : attributes.entrySet()) {
            if (e.getKey() != null && e.getKey().equalsIgnoreCase(name)) {
                return e.getValue();
            }
        }
        return null;
    }
}
