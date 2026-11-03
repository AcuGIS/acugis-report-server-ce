package com.acugis.reports;

import org.jfree.chart.JFreeChart;
import org.jfree.data.category.CategoryDataset;
import org.jfree.data.general.Dataset;
import org.jfree.data.general.PieDataset;
import org.jfree.data.xy.XYDataset;

import net.sf.jasperreports.charts.ChartContext;
import net.sf.jasperreports.charts.ChartTheme;
import net.sf.jasperreports.charts.JRChart;
import net.sf.jasperreports.charts.fill.DefaultChartTheme;
import net.sf.jasperreports.engine.JRException;
import net.sf.jasperreports.engine.type.EvaluationTimeEnum;

/**
 * Wraps {@link DefaultChartTheme#createChart} to time JFreeChart construction
 * during Jasper fill (dataset already collected).
 */
public final class TimingChartTheme implements ChartTheme {

    public static final String THEME_NAME = "acugis_timing";

    private final ChartTheme delegate = new DefaultChartTheme();

    @Override
    public JFreeChart createChart(ChartContext context) throws JRException {
        ChartFillProbe.begin();
        ChartFillProbe probe = ChartFillProbe.current();
        ChartRenderSession session = ChartRenderSession.currentOrBegin("");
        if (context != null && context.getChart() != null) {
            JRChart chart = context.getChart();
            probe.chartKey = chart.getKey() == null ? "" : chart.getKey();
            probe.chartType = chart.getChartType() == null ? "" : String.valueOf(chart.getChartType());
            probe.renderType = chart.getRenderType() == null ? "" : chart.getRenderType();
            EvaluationTimeEnum eval = chart.getEvaluationTime();
            probe.evaluationTime = eval == null ? "" : String.valueOf(eval);
            probe.datasetItemCount = countDatasetItems(context.getDataset());
            probe.evaluationCount = session.noteEvaluation(probe.chartKey + "|" + probe.chartType);
        }
        long start = System.nanoTime();
        try {
            return delegate.createChart(context);
        } finally {
            probe.createChartMs = (System.nanoTime() - start) / 1_000_000L;
        }
    }

    private static int countDatasetItems(Dataset dataset) {
        if (dataset == null) {
            return -1;
        }
        try {
            if (dataset instanceof CategoryDataset) {
                CategoryDataset cat = (CategoryDataset) dataset;
                int total = 0;
                for (int r = 0; r < cat.getRowCount(); r++) {
                    total += cat.getColumnCount();
                }
                return total;
            }
            if (dataset instanceof PieDataset) {
                return ((PieDataset<?>) dataset).getItemCount();
            }
            if (dataset instanceof XYDataset) {
                XYDataset xy = (XYDataset) dataset;
                int total = 0;
                for (int s = 0; s < xy.getSeriesCount(); s++) {
                    total += xy.getItemCount(s);
                }
                return total;
            }
        } catch (Throwable ignored) {
            // best-effort
        }
        return -1;
    }
}
