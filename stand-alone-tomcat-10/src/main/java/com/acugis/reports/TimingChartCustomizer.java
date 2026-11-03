package com.acugis.reports;

import org.jfree.chart.JFreeChart;

import net.sf.jasperreports.charts.JRAbstractChartCustomizer;
import net.sf.jasperreports.charts.JRChart;

/**
 * Global chart customizer used only for timing/metadata between createChart and
 * renderable factory. Does not alter chart appearance.
 */
public final class TimingChartCustomizer extends JRAbstractChartCustomizer {

    @Override
    public void customize(JFreeChart chart, JRChart jasperChart) {
        ChartFillProbe probe = ChartFillProbe.current();
        if (probe == null) {
            ChartFillProbe.begin();
            probe = ChartFillProbe.current();
            if (jasperChart != null) {
                probe.chartKey = jasperChart.getKey() == null ? "" : jasperChart.getKey();
                probe.chartType = jasperChart.getChartType() == null
                        ? "" : String.valueOf(jasperChart.getChartType());
                probe.renderType = jasperChart.getRenderType() == null ? "" : jasperChart.getRenderType();
            }
        }
        long start = System.nanoTime();
        // No visual customization — measure overhead only.
        probe.customizeMs = (System.nanoTime() - start) / 1_000_000L;
        if (chart != null && chart.getTitle() != null && (probe.chartKey == null || probe.chartKey.isEmpty())) {
            try {
                probe.chartKey = String.valueOf(chart.getTitle().getText());
            } catch (Throwable ignored) {
                // ignore
            }
        }
    }
}
