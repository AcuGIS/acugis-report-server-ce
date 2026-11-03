package com.acugis.map;

import java.awt.Color;
import java.util.ArrayList;
import java.util.LinkedHashMap;
import java.util.List;
import java.util.Locale;
import java.util.Map;
import org.json.JSONArray;
import org.json.JSONObject;

/**
 * Parsed symbology JSON (Server mapDefinition subset for Phase A+B+C).
 */
final class Symbology {

    enum Renderer {
        SINGLE,
        CATEGORIZED,
        GRADUATED,
        CHOROPLETH,
        HEATMAP,
        PROPORTIONAL
    }

    enum Classification {
        QUANTILE,
        EQUAL_INTERVAL,
        NATURAL_BREAKS,
        STANDARD_DEVIATION
    }

    static final class SingleSymbol {
        Color fill = new Color(0x26, 0xa6, 0x9a);
        Color outline = new Color(0x00, 0x79, 0x6b);
        float outlineWidth = 1.5f;
        float opacity = 0.35f;
        String shape = "circle";
    }

    static final class Category {
        String value;
        String label;
        Color fill;
        Color outline;
    }

    /** Provider-independent feature labels (parsed from style JSON "labels"). */
    static final class Labels {
        boolean enabled = false;
        String field = "";
        String fontFamily = "SansSerif";
        float fontSize = 10f;
        boolean bold = false;
        boolean italic = false;
        Color fontColor = Color.BLACK;
        boolean halo = false;
        Color haloColor = Color.WHITE;
        float haloWidth = 2f;
        String placement = "right";
        float offsetX = 0f;
        float offsetY = 0f;
        boolean avoidOverlap = false;
        double minScale = 0;
        double maxScale = 0;
    }

    Renderer renderer = Renderer.SINGLE;
    String field = "";
    Classification classification = Classification.QUANTILE;
    int classes = 5;
    String colorRamp = "Blues";
    SingleSymbol single = new SingleSymbol();
    List<Category> categories = new ArrayList<Category>();
    boolean showLegend = false;
    Labels labels = new Labels();

    static Symbology defaults() {
        return new Symbology();
    }

    static Symbology parse(String json) {
        Symbology s = defaults();
        if (json == null || json.trim().isEmpty()) {
            return s;
        }
        JSONObject o = new JSONObject(json);
        String r = o.optString("renderer", "single").trim().toLowerCase(Locale.US);
        if ("single_symbol".equals(r) || "single-symbol".equals(r)) {
            r = "single";
        }
        if ("proportional_symbols".equals(r) || "proportional-symbols".equals(r)) {
            r = "proportional";
        }
        if ("categorized".equals(r)) {
            s.renderer = Renderer.CATEGORIZED;
        } else if ("graduated".equals(r)) {
            s.renderer = Renderer.GRADUATED;
        } else if ("choropleth".equals(r)) {
            s.renderer = Renderer.CHOROPLETH;
        } else if ("heatmap".equals(r)) {
            s.renderer = Renderer.HEATMAP;
        } else if ("proportional".equals(r)) {
            s.renderer = Renderer.PROPORTIONAL;
        } else {
            s.renderer = Renderer.SINGLE;
        }
        s.field = o.optString("field", "").trim();
        String c = o.optString("classification", "quantile").trim().toLowerCase(Locale.US)
                .replace(' ', '_').replace('-', '_');
        if ("naturalbreaks".equals(c)) {
            c = "natural_breaks";
        }
        if ("equalinterval".equals(c)) {
            c = "equal_interval";
        }
        if ("stddev".equals(c) || "std_dev".equals(c)) {
            c = "standard_deviation";
        }
        if ("equal_interval".equals(c)) {
            s.classification = Classification.EQUAL_INTERVAL;
        } else if ("natural_breaks".equals(c)) {
            s.classification = Classification.NATURAL_BREAKS;
        } else if ("standard_deviation".equals(c)) {
            s.classification = Classification.STANDARD_DEVIATION;
        } else {
            s.classification = Classification.QUANTILE;
        }
        int n = o.optInt("classes", 5);
        s.classes = Math.max(2, Math.min(12, n > 0 ? n : 5));
        String ramp = o.optString("colorRamp", o.optString("color_ramp", "Blues"));
        s.colorRamp = ramp == null || ramp.isEmpty() ? "Blues" : ramp;
        s.showLegend = o.optBoolean("showLegend", o.optBoolean("show_legend", false));

        JSONObject labels = o.optJSONObject("labels");
        if (labels != null) {
            s.labels = parseLabels(labels);
        }

        JSONObject ss = o.optJSONObject("singleSymbol");
        if (ss == null) {
            ss = o.optJSONObject("single_symbol");
        }
        if (ss != null) {
            s.single.fill = parseColor(ss.optString("fillColor", ss.optString("fill", "#26a69a")), s.single.fill);
            s.single.outline = parseColor(
                    ss.optString("outlineColor", ss.optString("stroke", "#00796b")), s.single.outline);
            if (ss.has("outlineWidth") || ss.has("strokeWidth")) {
                s.single.outlineWidth = (float) Math.max(0, Math.min(20,
                        ss.optDouble("outlineWidth", ss.optDouble("strokeWidth", 1.5))));
            }
            if (ss.has("opacity") || ss.has("fillOpacity")) {
                s.single.opacity = (float) Math.max(0, Math.min(1,
                        ss.optDouble("opacity", ss.optDouble("fillOpacity", 0.35))));
            }
            String shape = ss.optString("shape", "circle");
            if (shape != null && !shape.isEmpty()) {
                s.single.shape = shape;
            }
        }

        JSONArray cats = o.optJSONArray("categories");
        if (cats != null) {
            for (int i = 0; i < cats.length(); i++) {
                JSONObject cat = cats.optJSONObject(i);
                if (cat == null) {
                    continue;
                }
                Object val = cat.opt("value");
                if (val == null || String.valueOf(val).isEmpty()) {
                    continue;
                }
                Category cobj = new Category();
                cobj.value = String.valueOf(val);
                cobj.label = cat.optString("label", cobj.value);
                cobj.fill = parseColor(cat.optString("fillColor", cat.optString("fill", "")), null);
                cobj.outline = parseColor(cat.optString("outlineColor", cat.optString("stroke", "")), null);
                s.categories.add(cobj);
            }
        }
        return s;
    }

    private static Labels parseLabels(JSONObject o) {
        Labels L = new Labels();
        L.enabled = o.optBoolean("enabled", false);
        L.field = o.optString("field", "").trim();
        String family = o.optString("fontFamily", o.optString("font_family", "SansSerif")).trim();
        if (!family.isEmpty()) {
            L.fontFamily = family;
        }
        if (o.has("fontSize") || o.has("font_size")) {
            L.fontSize = (float) Math.max(4, Math.min(72,
                    o.optDouble("fontSize", o.optDouble("font_size", 10))));
        }
        L.bold = o.optBoolean("bold", false);
        L.italic = o.optBoolean("italic", false);
        L.fontColor = parseColor(
                o.optString("fontColor", o.optString("font_color", "#000000")), Color.BLACK);
        L.halo = o.optBoolean("halo", false);
        L.haloColor = parseColor(
                o.optString("haloColor", o.optString("halo_color", "#FFFFFF")), Color.WHITE);
        if (o.has("haloWidth") || o.has("halo_width")) {
            L.haloWidth = (float) Math.max(0, Math.min(20,
                    o.optDouble("haloWidth", o.optDouble("halo_width", 2))));
        }
        String place = o.optString("placement", "right").trim().toLowerCase(Locale.US)
                .replace(' ', '_').replace('-', '_');
        if ("upperleft".equals(place) || "upper_left".equals(place)) {
            place = "upperLeft";
        } else if ("upperright".equals(place) || "upper_right".equals(place)) {
            place = "upperRight";
        } else if ("lowerleft".equals(place) || "lower_left".equals(place)) {
            place = "lowerLeft";
        } else if ("lowerright".equals(place) || "lower_right".equals(place)) {
            place = "lowerRight";
        }
        if ("center".equals(place) || "above".equals(place) || "below".equals(place)
                || "left".equals(place) || "right".equals(place)
                || "upperLeft".equals(place) || "upperRight".equals(place)
                || "lowerLeft".equals(place) || "lowerRight".equals(place)) {
            L.placement = place;
        }
        L.offsetX = (float) o.optDouble("offsetX", o.optDouble("offset_x", 0));
        L.offsetY = (float) o.optDouble("offsetY", o.optDouble("offset_y", 0));
        L.avoidOverlap = o.optBoolean("avoidOverlap", o.optBoolean("avoid_overlap", false));
        L.minScale = o.optDouble("minScale", o.optDouble("min_scale", 0));
        L.maxScale = o.optDouble("maxScale", o.optDouble("max_scale", 0));
        return L;
    }

    boolean isThematic() {
        return renderer == Renderer.CATEGORIZED
                || renderer == Renderer.GRADUATED
                || renderer == Renderer.CHOROPLETH
                || renderer == Renderer.HEATMAP
                || renderer == Renderer.PROPORTIONAL;
    }

    Color withOpacity(Color base) {
        if (base == null) {
            base = single.fill;
        }
        int a = Math.round(single.opacity * 255f);
        a = Math.max(0, Math.min(255, a));
        return new Color(base.getRed(), base.getGreen(), base.getBlue(), a);
    }

    static Color parseColor(String hex, Color fallback) {
        if (hex == null || hex.trim().isEmpty()) {
            return fallback;
        }
        String h = hex.trim();
        if (h.startsWith("#")) {
            h = h.substring(1);
        }
        try {
            if (h.length() == 3) {
                h = "" + h.charAt(0) + h.charAt(0) + h.charAt(1) + h.charAt(1) + h.charAt(2) + h.charAt(2);
            }
            int n = Integer.parseInt(h, 16);
            return new Color((n >> 16) & 255, (n >> 8) & 255, n & 255);
        } catch (Exception e) {
            return fallback;
        }
    }

    static String toHex(Color c) {
        if (c == null) {
            return "#9e9e9e";
        }
        return String.format("#%02x%02x%02x", c.getRed(), c.getGreen(), c.getBlue());
    }

    static Map<String, String[]> COLOR_RAMPS() {
        Map<String, String[]> m = new LinkedHashMap<String, String[]>();
        m.put("Blues", new String[] {"#f7fbff", "#c6dbef", "#6baed6", "#2171b5", "#08306b"});
        m.put("Greens", new String[] {"#f7fcf5", "#c7e9c0", "#74c476", "#238b45", "#00441b"});
        m.put("Reds", new String[] {"#fff5f0", "#fcbba1", "#fb6a4a", "#cb181d", "#67000d"});
        m.put("Viridis", new String[] {"#440154", "#3b528b", "#21918c", "#5ec962", "#fde725"});
        m.put("Spectral", new String[] {"#9e0142", "#f46d43", "#fee08b", "#66c2a5", "#5e4fa2"});
        return m;
    }

    /** Classic heat ramp (cool → hot), used by heatmap renderer. */
    static final String[] HEAT_RAMP = new String[] {
        "#0000ff", "#00ffff", "#00ff00", "#ffff00", "#ff0000"
    };

    List<Color> rampColors(int n) {
        String[] base = COLOR_RAMPS().get(colorRamp);
        if (base == null) {
            base = COLOR_RAMPS().get("Blues");
        }
        return interpolateRamp(base, n);
    }

    static List<Color> heatRampColors(int n) {
        return interpolateRamp(HEAT_RAMP, n);
    }

    private static List<Color> interpolateRamp(String[] base, int n) {
        n = Math.max(2, Math.min(12, n));
        List<Color> out = new ArrayList<Color>(n);
        for (int i = 0; i < n; i++) {
            double t = n == 1 ? 0 : (double) i / (double) (n - 1);
            double idx = t * (base.length - 1);
            int lo = (int) Math.floor(idx);
            int hi = Math.min(base.length - 1, lo + 1);
            Color clo = parseColor(base[lo], Color.GRAY);
            Color chi = parseColor(base[hi], Color.GRAY);
            if (lo == hi) {
                out.add(clo);
            } else {
                float f = (float) (idx - lo);
                out.add(new Color(
                        Math.round(clo.getRed() + (chi.getRed() - clo.getRed()) * f),
                        Math.round(clo.getGreen() + (chi.getGreen() - clo.getGreen()) * f),
                        Math.round(clo.getBlue() + (chi.getBlue() - clo.getBlue()) * f)));
            }
        }
        return out;
    }
}
