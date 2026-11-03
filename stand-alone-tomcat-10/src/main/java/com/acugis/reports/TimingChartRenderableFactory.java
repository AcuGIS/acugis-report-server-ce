package com.acugis.reports;

import java.awt.Graphics2D;
import java.awt.geom.Rectangle2D;
import java.util.List;

import org.jfree.chart.JFreeChart;

import net.sf.jasperreports.charts.util.ChartHyperlinkProvider;
import net.sf.jasperreports.charts.util.ChartRenderableFactory;
import net.sf.jasperreports.charts.util.DrawChartRendererFactory;
import net.sf.jasperreports.charts.util.ImageChartRendererFactory;
import net.sf.jasperreports.charts.util.SvgChartRendererFactory;
import net.sf.jasperreports.engine.JRException;
import net.sf.jasperreports.engine.JRPrintImageAreaHyperlink;
import net.sf.jasperreports.engine.JasperReportsContext;
import net.sf.jasperreports.renderers.AbstractRenderer;
import net.sf.jasperreports.renderers.AreaHyperlinksRenderable;
import net.sf.jasperreports.renderers.Graphics2DRenderable;
import net.sf.jasperreports.renderers.Renderable;

/**
 * Wraps Jasper chart renderable factories to time factory creation and deferred
 * {@link Graphics2DRenderable#render} (HTML/PDF image trick path).
 */
public class TimingChartRenderableFactory implements ChartRenderableFactory {

    public enum Kind {
        DRAW, IMAGE, SVG
    }

    private final ChartRenderableFactory delegate;
    private final Kind kind;

    public TimingChartRenderableFactory() {
        // Jasper singleton cache may instantiate via no-arg ctor for the configured class.
        // Default to draw (Jasper default render type).
        this(Kind.DRAW);
    }

    public TimingChartRenderableFactory(Kind kind) {
        this.kind = kind == null ? Kind.DRAW : kind;
        switch (this.kind) {
            case IMAGE:
                this.delegate = new ImageChartRendererFactory();
                break;
            case SVG:
                this.delegate = new SvgChartRendererFactory();
                break;
            case DRAW:
            default:
                this.delegate = new DrawChartRendererFactory();
                break;
        }
    }

    /** Explicit factories for property wiring (avoid ambiguous no-arg reuse). */
    public static final class Draw extends TimingChartRenderableFactory {
        public Draw() {
            super(Kind.DRAW);
        }
    }

    public static final class Image extends TimingChartRenderableFactory {
        public Image() {
            super(Kind.IMAGE);
        }
    }

    public static final class Svg extends TimingChartRenderableFactory {
        public Svg() {
            super(Kind.SVG);
        }
    }

    @Override
    public Renderable getRenderable(
            JasperReportsContext jasperReportsContext,
            JFreeChart chart,
            ChartHyperlinkProvider chartHyperlinkProvider,
            Rectangle2D rectangle) {
        ChartFillProbe probe = ChartFillProbe.current();
        long start = System.nanoTime();
        Renderable renderable = delegate.getRenderable(
                jasperReportsContext, chart, chartHyperlinkProvider, rectangle);
        long factoryMs = (System.nanoTime() - start) / 1_000_000L;
        if (probe != null) {
            probe.factoryMs = factoryMs;
            probe.renderType = kind.name().toLowerCase();
        }

        int width = rectangle == null ? 0 : (int) Math.round(rectangle.getWidth());
        int height = rectangle == null ? 0 : (int) Math.round(rectangle.getHeight());

        ChartRenderSession session = ChartRenderSession.currentOrBegin("");
        if (probe != null) {
            long totalMs = (System.nanoTime() - probe.startedNanos) / 1_000_000L;
            session.recordCreate(probe.createChartMs, probe.customizeMs, probe.factoryMs, totalMs);
            ChartPerfRecord rec = new ChartPerfRecord();
            rec.reportName = session.reportName();
            rec.chartKey = probe.chartKey;
            rec.chartType = probe.chartType;
            rec.renderType = probe.renderType;
            rec.evaluationTime = probe.evaluationTime;
            rec.evaluationCount = probe.evaluationCount;
            rec.datasetItemCount = probe.datasetItemCount;
            rec.width = width;
            rec.height = height;
            rec.createChartMs = probe.createChartMs;
            rec.customizeMs = probe.customizeMs;
            rec.factoryMs = probe.factoryMs;
            rec.totalMs = totalMs;
            rec.phase = "fill";
            rec.log();
        }

        if (renderable instanceof Graphics2DRenderable) {
            String chartKey = probe == null ? "" : probe.chartKey;
            String chartType = probe == null ? "" : probe.chartType;
            int evalCount = probe == null ? 0 : probe.evaluationCount;
            ChartFillProbe.clear();
            return new TimingGraphics2DRenderable(
                    (Graphics2DRenderable) renderable, chartKey, chartType, evalCount, width, height);
        }
        ChartFillProbe.clear();
        return renderable;
    }

    /**
     * Times deferred chart drawing used by HTML/PDF exporters (same image delivery
     * path as maps/QR via CapturingHtmlResourceHandler).
     */
    private static final class TimingGraphics2DRenderable extends AbstractRenderer
            implements Graphics2DRenderable, AreaHyperlinksRenderable {

        private final Graphics2DRenderable delegate;
        private final AreaHyperlinksRenderable hyperlinks;
        private final String chartKey;
        private final String chartType;
        private final int evaluationCount;
        private final int width;
        private final int height;

        TimingGraphics2DRenderable(
                Graphics2DRenderable delegate,
                String chartKey,
                String chartType,
                int evaluationCount,
                int width,
                int height) {
            this.delegate = delegate;
            this.hyperlinks = delegate instanceof AreaHyperlinksRenderable
                    ? (AreaHyperlinksRenderable) delegate : null;
            this.chartKey = chartKey == null ? "" : chartKey;
            this.chartType = chartType == null ? "" : chartType;
            this.evaluationCount = evaluationCount;
            this.width = width;
            this.height = height;
        }

        @Override
        public void render(JasperReportsContext jasperReportsContext, Graphics2D grx, Rectangle2D rectangle)
                throws JRException {
            long start = System.nanoTime();
            try {
                delegate.render(jasperReportsContext, grx, rectangle);
            } finally {
                long ms = (System.nanoTime() - start) / 1_000_000L;
                ChartRenderSession session = ChartRenderSession.current();
                if (session != null) {
                    session.recordDraw(ms);
                }
                if (PerformanceProfiler.isEnabled()) {
                    ChartPerfRecord rec = new ChartPerfRecord();
                    if (session != null) {
                        rec.reportName = session.reportName();
                    }
                    rec.chartKey = chartKey;
                    rec.chartType = chartType;
                    rec.evaluationCount = evaluationCount;
                    rec.width = width;
                    rec.height = height;
                    rec.drawMs = ms;
                    rec.totalMs = ms;
                    rec.phase = "draw";
                    rec.log();
                }
            }
        }

        @Override
        public List<JRPrintImageAreaHyperlink> getImageAreaHyperlinks(Rectangle2D renderingArea)
                throws JRException {
            if (hyperlinks == null) {
                return null;
            }
            return hyperlinks.getImageAreaHyperlinks(renderingArea);
        }

        @Override
        public boolean hasImageAreaHyperlinks() {
            return hyperlinks != null && hyperlinks.hasImageAreaHyperlinks();
        }
    }
}
