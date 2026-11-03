package com.acugis.reports;

import java.util.Collections;
import java.util.LinkedHashMap;
import java.util.Map;
import java.util.logging.Level;
import java.util.logging.Logger;

import net.sf.jasperreports.engine.JRFont;
import net.sf.jasperreports.engine.JRStaticText;
import net.sf.jasperreports.engine.JRStyle;
import net.sf.jasperreports.engine.JRTextField;
import net.sf.jasperreports.engine.JasperReport;
import net.sf.jasperreports.engine.util.JRElementsVisitor;
import net.sf.jasperreports.engine.util.JRVisitorSupport;

/**
 * Remaps legacy / OS font names in a compiled report onto bundled JasperReports
 * font families so JRXML that references e.g. {@code Times New Roman} renders
 * without requiring host OS fonts.
 *
 * <p>Alias families are also registered in {@code fonts/fonts.xml}; this visitor
 * rewrites design {@code fontName} attributes onto the canonical bundled family
 * for consistent PDF embedding and HTML/XLSX/DOCX export CSS fallbacks.
 */
public final class FontNameAliaser {

    private static final Logger LOG = Logger.getLogger(FontNameAliaser.class.getName());

    /**
     * Legacy / logical font name → bundled family name.
     * Keys compared case-insensitively.
     */
    public static final Map<String, String> ALIASES;

    static {
        Map<String, String> m = new LinkedHashMap<>();
        // Sans
        m.put("Arial", "Liberation Sans");
        m.put("Helvetica", "Nimbus Sans");
        m.put("Helvetica Neue", "Nimbus Sans");
        m.put("Verdana", "DejaVu Sans");
        m.put("Tahoma", "Liberation Sans");
        m.put("Calibri", "Carlito");
        m.put("Aptos", "Carlito");
        m.put("SansSerif", "DejaVu Sans");
        m.put("Dialog", "DejaVu Sans");
        // Serif
        m.put("Times", "Nimbus Roman");
        m.put("Times New Roman", "Liberation Serif");
        m.put("Georgia", "Liberation Serif");
        m.put("Cambria", "Caladea");
        m.put("Serif", "DejaVu Serif");
        // Monospace
        m.put("Courier", "Liberation Mono");
        m.put("Courier New", "Liberation Mono");
        m.put("Consolas", "Cousine");
        m.put("Monospaced", "DejaVu Sans Mono");
        m.put("DialogInput", "DejaVu Sans Mono");
        ALIASES = Collections.unmodifiableMap(m);
    }

    private FontNameAliaser() {
    }

    /** Resolve a report font name to its bundled family, or return the original. */
    public static String resolve(String fontName) {
        if (fontName == null || fontName.isEmpty()) {
            return fontName;
        }
        String mapped = ALIASES.get(fontName);
        if (mapped != null) {
            return mapped;
        }
        for (Map.Entry<String, String> e : ALIASES.entrySet()) {
            if (e.getKey().equalsIgnoreCase(fontName)) {
                return e.getValue();
            }
        }
        return fontName;
    }

    /**
     * Rewrite font names on styles and text elements of a compiled report.
     * Safe to call multiple times; already-canonical names are left unchanged.
     *
     * @return number of fontName attributes remapped
     */
    public static int apply(JasperReport report) {
        if (report == null) {
            return 0;
        }
        int[] count = {0};
        try {
            JRStyle[] styles = report.getStyles();
            if (styles != null) {
                for (JRStyle style : styles) {
                    count[0] += remapStyle(style);
                }
            }
            JRStyle defaultStyle = report.getDefaultStyle();
            if (defaultStyle != null) {
                count[0] += remapStyle(defaultStyle);
            }

            JRElementsVisitor.visitReport(report, new JRVisitorSupport() {
                @Override
                public void visitTextField(JRTextField textField) {
                    count[0] += remapFont(textField);
                }

                @Override
                public void visitStaticText(JRStaticText staticText) {
                    count[0] += remapFont(staticText);
                }
            });
        } catch (Throwable t) {
            LOG.log(Level.WARNING, "Font name aliasing failed; continuing with extension aliases only", t);
        }
        if (count[0] > 0 && LOG.isLoggable(Level.FINE)) {
            LOG.fine("Remapped " + count[0] + " font name(s) on report "
                    + (report.getName() != null ? report.getName() : "(unnamed)"));
        }
        return count[0];
    }

    private static int remapStyle(JRStyle style) {
        if (style == null) {
            return 0;
        }
        return remap(style.getOwnFontName(), style::setFontName);
    }

    private static int remapFont(JRFont font) {
        if (font == null) {
            return 0;
        }
        return remap(font.getOwnFontName(), font::setFontName);
    }

    private static int remap(String own, java.util.function.Consumer<String> setter) {
        if (own == null || own.isEmpty()) {
            return 0;
        }
        String resolved = resolve(own);
        if (own.equals(resolved)) {
            return 0;
        }
        try {
            setter.accept(resolved);
            return 1;
        } catch (UnsupportedOperationException | IllegalStateException ex) {
            if (LOG.isLoggable(Level.FINE)) {
                LOG.fine("Could not set fontName '" + own + "' → '" + resolved + "': " + ex.getMessage());
            }
            return 0;
        }
    }

    /** Diagnostic: describe the alias table for startup logs. */
    public static String describeAliases() {
        StringBuilder sb = new StringBuilder();
        for (Map.Entry<String, String> e : ALIASES.entrySet()) {
            if (sb.length() > 0) {
                sb.append(", ");
            }
            sb.append(e.getKey()).append("→").append(e.getValue());
        }
        return sb.toString();
    }
}
