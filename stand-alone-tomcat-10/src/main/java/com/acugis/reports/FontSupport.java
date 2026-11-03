package com.acugis.reports;

import java.awt.GraphicsEnvironment;
import java.io.File;
import java.io.InputStream;
import java.util.ArrayList;
import java.util.Collection;
import java.util.LinkedHashSet;
import java.util.List;
import java.util.Set;
import java.util.logging.Level;
import java.util.logging.Logger;

import net.sf.jasperreports.engine.DefaultJasperReportsContext;
import net.sf.jasperreports.engine.JasperReportsContext;
import net.sf.jasperreports.engine.fonts.FontUtil;

/**
 * Configures JasperReports bundled font extension, legacy font-name aliases,
 * and optional operator fonts under {@code fonts.dir}.
 *
 * <p>Bundled families live on the classpath at {@code fonts/} and are registered
 * via {@code jasperreports_extension.properties}. No host OS fonts are required.
 */
public final class FontSupport {

    private static final Logger LOG = Logger.getLogger(FontSupport.class.getName());

    /** Canonical families expected from the bundled extension (visible families). */
    private static final String[] BUNDLED_FAMILIES = {
            "DejaVu Sans", "DejaVu Serif", "DejaVu Sans Mono",
            "Liberation Sans", "Liberation Serif", "Liberation Mono",
            "Nimbus Sans", "Nimbus Roman", "Nimbus Mono PS",
            "Carlito", "Caladea", "Cousine",
            "Noto Sans", "Noto Serif"
    };

    /** Representative classpath font files used for startup existence checks. */
    private static final String[] BUNDLED_FONT_FILES = {
            "fonts/dejavu/DejaVuSans.ttf",
            "fonts/liberation/LiberationSans-Regular.ttf",
            "fonts/liberation/LiberationSerif-Regular.ttf",
            "fonts/liberation/LiberationMono-Regular.ttf",
            "fonts/nimbus/NimbusSans-Regular.otf",
            "fonts/nimbus/NimbusRoman-Regular.otf",
            "fonts/nimbus/NimbusMonoPS-Regular.otf",
            "fonts/carlito/Carlito-Regular.ttf",
            "fonts/caladea/Caladea-Regular.ttf",
            "fonts/cousine/Cousine-Regular.ttf",
            "fonts/noto/NotoSans-Regular.ttf",
            "fonts/noto/NotoSerif-Regular.ttf",
            "fonts/fonts.xml"
    };

    private FontSupport() {
    }

    public static final class Result {
        public final List<String> availableFamilies;
        public final List<String> bundledFamilies;
        public final File fontsDir;
        public final boolean ignoreMissingFont;
        public final String message;

        Result(List<String> availableFamilies, List<String> bundledFamilies, File fontsDir,
                boolean ignoreMissingFont, String message) {
            this.availableFamilies = availableFamilies;
            this.bundledFamilies = bundledFamilies;
            this.fontsDir = fontsDir;
            this.ignoreMissingFont = ignoreMissingFont;
            this.message = message;
        }
    }

    public static Result configure() {
        AcugisConfig cfg = AcugisConfig.get();
        boolean ignoreMissing = Boolean.parseBoolean(cfg.get("ignore.missing.font", "true"));
        DefaultJasperReportsContext ctx = DefaultJasperReportsContext.getInstance();

        System.setProperty("net.sf.jasperreports.awt.ignore.missing.font", String.valueOf(ignoreMissing));
        ctx.setProperty("net.sf.jasperreports.awt.ignore.missing.font", String.valueOf(ignoreMissing));

        applyPropertyAliases(ctx);

        File fontsDir = cfg.fontsDir();
        if (!fontsDir.exists()) {
            // Non-fatal — bundled classpath fonts remain available.
            //noinspection ResultOfMethodCallIgnored
            fontsDir.mkdirs();
        }
        if (fontsDir.isDirectory()) {
            System.setProperty("acugis.fonts.dir", fontsDir.getAbsolutePath());
            File fontsXml = new File(fontsDir, "fonts.xml");
            if (fontsXml.isFile()) {
                // Optional operator-supplied families (in addition to the WAR extension).
                System.setProperty("net.sf.jasperreports.extension.simple.font.families.acugis.external",
                        fontsXml.toURI().toString());
                ctx.setProperty("net.sf.jasperreports.extension.simple.font.families.acugis.external",
                        fontsXml.toURI().toString());
            }
        }

        validateClasspathFonts();

        List<String> bundled = new ArrayList<>();
        List<String> allFamilies = new ArrayList<>();
        try {
            FontUtil fontUtil = FontUtil.getInstance(ctx);
            Collection<String> names = fontUtil.getFontFamilyNames();
            if (names != null) {
                Set<String> ordered = new LinkedHashSet<>(names);
                allFamilies.addAll(ordered);
                for (String expected : BUNDLED_FAMILIES) {
                    if (ordered.contains(expected) || fontUtil.getFontInfo(expected, null) != null) {
                        bundled.add(expected);
                    } else {
                        LOG.warning("Bundled font family not registered: " + expected);
                    }
                }
                // Alias families use visible=false so they may not appear in getFontFamilyNames().
                for (String alias : FontNameAliaser.ALIASES.keySet()) {
                    if (fontUtil.getFontInfo(alias, null) == null) {
                        LOG.warning("Font alias not available to JasperReports: " + alias
                                + " (expected → " + FontNameAliaser.resolve(alias) + ")");
                    }
                }
            }
            LOG.info("JasperReports font families loaded (" + allFamilies.size() + "): " + allFamilies);
            LOG.info("Bundled font families (" + bundled.size() + "/" + BUNDLED_FAMILIES.length + "): " + bundled);
            LOG.info("Font name aliases: " + FontNameAliaser.describeAliases());
        } catch (Throwable t) {
            LOG.log(Level.WARNING, "Unable to enumerate JasperReports font families; continuing startup", t);
        }

        // AWT enumeration is informational only (headless / OS fonts).
        try {
            GraphicsEnvironment ge = GraphicsEnvironment.getLocalGraphicsEnvironment();
            String[] awtNames = ge.getAvailableFontFamilyNames();
            if (awtNames != null && LOG.isLoggable(Level.FINE)) {
                LOG.fine("AWT font families available to JVM: " + awtNames.length);
            }
        } catch (Throwable t) {
            LOG.log(Level.FINE, "Unable to enumerate AWT font families (headless)", t);
        }

        String message = "Fonts ready; bundled=" + bundled.size() + "/" + BUNDLED_FAMILIES.length
                + "; jrFamilies=" + allFamilies.size()
                + "; aliases=" + FontNameAliaser.ALIASES.size()
                + "; ignore.missing.font=" + ignoreMissing
                + "; fonts.dir=" + (fontsDir != null ? fontsDir.getAbsolutePath() : "(null)");
        return new Result(allFamilies, bundled, fontsDir, ignoreMissing, message);
    }

    /**
     * Also publish alias remaps as JR context/system properties for any code path
     * that honors {@code net.sf.jasperreports.font.family.*}.
     */
    private static void applyPropertyAliases(JasperReportsContext ctx) {
        for (MapEntry e : propertyAliasEntries()) {
            String key = "net.sf.jasperreports.font.family." + e.from;
            if (System.getProperty(key) == null) {
                System.setProperty(key, e.to);
            }
            if (ctx.getProperty(key) == null) {
                ctx.setProperty(key, e.to);
            }
        }
    }

    private static List<MapEntry> propertyAliasEntries() {
        List<MapEntry> list = new ArrayList<>();
        for (java.util.Map.Entry<String, String> e : FontNameAliaser.ALIASES.entrySet()) {
            list.add(new MapEntry(e.getKey(), e.getValue()));
        }
        return list;
    }

    private static final class MapEntry {
        final String from;
        final String to;

        MapEntry(String from, String to) {
            this.from = from;
            this.to = to;
        }
    }

    private static void validateClasspathFonts() {
        ClassLoader cl = FontSupport.class.getClassLoader();
        for (String path : BUNDLED_FONT_FILES) {
            try (InputStream in = cl != null ? cl.getResourceAsStream(path) : null) {
                if (in == null) {
                    LOG.warning("Bundled font resource missing from classpath: " + path);
                }
            } catch (Throwable t) {
                LOG.log(Level.WARNING, "Failed to open bundled font resource: " + path, t);
            }
        }
    }
}
