package com.acugis.reports;

import java.io.File;
import java.util.logging.Level;
import java.util.logging.Logger;

import net.sf.jasperreports.engine.DefaultJasperReportsContext;

/**
 * Configures JasperReports expression compilation for in-container JRXML loads.
 * JasperReports 7 requires jasperreports-jdt (ECJ); without it the JDK javac
 * fallback cannot see WEB-INF/lib and fails with missing JREvaluator.
 */
public final class CompilerSupport {

    private static final Logger LOG = Logger.getLogger(CompilerSupport.class.getName());

    /** Selects raster PNG producers (alias "image") for Barcode4J and QRCode. */
    private static final String BARCODE_IMAGE_PRODUCER =
            "net.sf.jasperreports.components.barcode4j.image.producer";

    private CompilerSupport() {
    }

    public static void configure() {
        AcugisConfig cfg = AcugisConfig.get();
        File tempDir = cfg.tempDir();
        if (tempDir != null) {
            if (!tempDir.isDirectory()) {
                //noinspection ResultOfMethodCallIgnored
                tempDir.mkdirs();
            }
            if (tempDir.isDirectory()) {
                System.setProperty("net.sf.jasperreports.compiler.temp.dir", tempDir.getAbsolutePath());
            }
        }

        try {
            Class.forName("net.sf.jasperreports.jdt.JRJdtCompiler");
            System.setProperty("net.sf.jasperreports.compiler.class", "net.sf.jasperreports.jdt.JRJdtCompiler");
            LOG.info("JasperReports JDT compiler available (jasperreports-jdt)");
        } catch (ClassNotFoundException e) {
            LOG.log(Level.WARNING,
                    "jasperreports-jdt not on classpath — JRXML compile will use JDK javac "
                            + "and usually fail inside Tomcat. Rebuild the WAR with jasperreports-jdt.",
                    e);
        }

        // Prefer raster PNG for Barcode4J / QR in HTML (same path as charts via
        // CapturingHtmlResourceHandler). Default SVG producers emit img_*.svg;
        // ReportImageServlet used to label those as image/png, which browsers
        // reject under X-Content-Type-Options: nosniff (Go + PHP viewers).
        // QR codes use the same image.producer alias selector as linear barcodes.
        System.setProperty(BARCODE_IMAGE_PRODUCER, "image");
        DefaultJasperReportsContext.getInstance().setProperty(BARCODE_IMAGE_PRODUCER, "image");
        LOG.info("Barcode4J/QR image producer set to raster PNG (image)");

        configureChartInstrumentation();
    }

    /**
     * Wire chart fill/export timing without changing chart visuals:
     * timing theme + global customizer + wrapped renderable factories.
     */
    private static void configureChartInstrumentation() {
        DefaultJasperReportsContext ctx = DefaultJasperReportsContext.getInstance();
        // When chart theme is unset, JRFillChart reads this property.
        ctx.setProperty("net.sf.jasperreports.chart.theme", TimingChartTheme.THEME_NAME);
        System.setProperty("net.sf.jasperreports.chart.theme", TimingChartTheme.THEME_NAME);

        // Global customizer (metadata / customize timing). Suffix is the customizer name.
        String customizerProp = "net.sf.jasperreports.customizer.class.acugis_perf";
        ctx.setProperty(customizerProp, TimingChartCustomizer.class.getName());
        System.setProperty(customizerProp, TimingChartCustomizer.class.getName());

        // Wrap all chart renderable factories (default render type is "draw").
        setFactory(ctx, "draw", TimingChartRenderableFactory.Draw.class.getName());
        setFactory(ctx, "image", TimingChartRenderableFactory.Image.class.getName());
        setFactory(ctx, "svg", TimingChartRenderableFactory.Svg.class.getName());
        LOG.info("Chart instrumentation enabled (theme=" + TimingChartTheme.THEME_NAME
                + ", timing factories for draw/image/svg)");
    }

    private static void setFactory(DefaultJasperReportsContext ctx, String renderType, String className) {
        String key = "net.sf.jasperreports.chart.renderer.factory." + renderType;
        ctx.setProperty(key, className);
        System.setProperty(key, className);
    }
}
