package com.acugis.map;

import java.awt.BasicStroke;
import java.awt.Color;
import java.awt.Font;
import java.awt.FontMetrics;
import java.awt.Graphics2D;
import java.awt.RenderingHints;
import java.awt.geom.Ellipse2D;
import java.awt.geom.RoundRectangle2D;
import java.util.ArrayList;
import java.util.List;

/** Draws an inset legend for thematic map renderers (Phase C). */
final class MapLegend {

    private static final int PAD = 8;
    private static final int ROW_H = 16;
    private static final int SWATCH = 12;
    private static final int MARGIN = 10;

    private MapLegend() {}

    static void paint(Graphics2D g, int imgW, int imgH, LegendModel model) {
        if (model == null || model.rows.isEmpty()) {
            return;
        }
        Font titleFont = new Font("SansSerif", Font.BOLD, 11);
        Font rowFont = new Font("SansSerif", Font.PLAIN, 10);
        FontMetrics tm = g.getFontMetrics(titleFont);
        FontMetrics rm = g.getFontMetrics(rowFont);

        int maxLabelW = tm.stringWidth(model.title);
        for (LegendRow row : model.rows) {
            maxLabelW = Math.max(maxLabelW, rm.stringWidth(row.label));
        }
        int symbolCol = model.maxSymbolPx > 0 ? model.maxSymbolPx : SWATCH;
        int boxW = PAD * 2 + symbolCol + 6 + maxLabelW;
        int boxH = PAD * 2 + tm.getHeight() + 4 + model.rows.size() * ROW_H;
        if (model.rampBar) {
            boxH = PAD * 2 + tm.getHeight() + 4 + 22;
            boxW = Math.max(boxW, 120);
        }

        int x = imgW - boxW - MARGIN;
        int y = MARGIN;
        if (x < MARGIN) {
            x = MARGIN;
        }

        g.setRenderingHint(RenderingHints.KEY_ANTIALIASING, RenderingHints.VALUE_ANTIALIAS_ON);
        g.setColor(new Color(255, 255, 255, 230));
        g.fill(new RoundRectangle2D.Double(x, y, boxW, boxH, 6, 6));
        g.setColor(new Color(0x90, 0xa4, 0xae));
        g.setStroke(new BasicStroke(1f));
        g.draw(new RoundRectangle2D.Double(x, y, boxW, boxH, 6, 6));

        int cy = y + PAD + tm.getAscent();
        g.setFont(titleFont);
        g.setColor(new Color(0x37, 0x47, 0x4f));
        g.drawString(model.title, x + PAD, cy);
        cy += tm.getDescent() + 6;

        if (model.rampBar) {
            int barX = x + PAD;
            int barY = cy;
            int barW = boxW - PAD * 2;
            int barH = 10;
            List<Color> ramp = Symbology.heatRampColors(5);
            for (int i = 0; i < barW; i++) {
                float t = barW <= 1 ? 0 : (float) i / (float) (barW - 1);
                int idx = Math.min(ramp.size() - 1, Math.round(t * (ramp.size() - 1)));
                g.setColor(ramp.get(idx));
                g.fillRect(barX + i, barY, 1, barH);
            }
            g.setColor(new Color(0x90, 0xa4, 0xae));
            g.drawRect(barX, barY, barW, barH);
            g.setFont(rowFont);
            g.setColor(new Color(0x45, 0x5a, 0x64));
            String lo = model.rows.isEmpty() ? "—" : model.rows.get(0).label;
            String hi = model.rows.size() < 2 ? "—" : model.rows.get(1).label;
            g.drawString(lo, barX, barY + barH + rm.getAscent() + 2);
            int hiW = rm.stringWidth(hi);
            g.drawString(hi, barX + barW - hiW, barY + barH + rm.getAscent() + 2);
            return;
        }

        g.setFont(rowFont);
        for (LegendRow row : model.rows) {
            int sy = cy;
            if (row.circle) {
                double d = Math.max(4, row.radius * 2);
                double cx = x + PAD + symbolCol / 2.0;
                double cyC = sy + ROW_H / 2.0;
                g.setColor(row.fill);
                g.fill(new Ellipse2D.Double(cx - d / 2, cyC - d / 2, d, d));
                g.setColor(row.stroke != null ? row.stroke : new Color(0x42, 0x42, 0x42));
                g.setStroke(new BasicStroke(1f));
                g.draw(new Ellipse2D.Double(cx - d / 2, cyC - d / 2, d, d));
            } else {
                int sx = x + PAD + (symbolCol - SWATCH) / 2;
                int swY = sy + (ROW_H - SWATCH) / 2;
                g.setColor(row.fill);
                g.fillRect(sx, swY, SWATCH, SWATCH);
                g.setColor(new Color(0x42, 0x42, 0x42));
                g.drawRect(sx, swY, SWATCH, SWATCH);
            }
            g.setColor(new Color(0x37, 0x47, 0x4f));
            g.drawString(row.label, x + PAD + symbolCol + 6, sy + rm.getAscent() + 2);
            cy += ROW_H;
        }
    }

    static final class LegendRow {
        final String label;
        final Color fill;
        final Color stroke;
        final boolean circle;
        final double radius;

        LegendRow(String label, Color fill, Color stroke, boolean circle, double radius) {
            this.label = label == null ? "" : label;
            this.fill = fill == null ? Color.GRAY : fill;
            this.stroke = stroke;
            this.circle = circle;
            this.radius = radius;
        }

        static LegendRow swatch(String label, Color fill) {
            return new LegendRow(label, fill, null, false, 0);
        }

        static LegendRow circle(String label, Color fill, Color stroke, double radius) {
            return new LegendRow(label, fill, stroke, true, radius);
        }
    }

    static final class LegendModel {
        String title = "";
        final List<LegendRow> rows = new ArrayList<LegendRow>();
        boolean rampBar = false;
        int maxSymbolPx = SWATCH;

        void add(LegendRow row) {
            rows.add(row);
            if (row.circle) {
                maxSymbolPx = Math.max(maxSymbolPx, (int) Math.ceil(row.radius * 2) + 2);
            }
        }
    }

    static String formatNumber(double n) {
        if (Double.isNaN(n) || Double.isInfinite(n)) {
            return "—";
        }
        double abs = Math.abs(n);
        if (abs >= 1_000_000) {
            String s = String.format(java.util.Locale.US, "%.1f", n / 1_000_000);
            if (s.endsWith(".0")) {
                s = s.substring(0, s.length() - 2);
            }
            return s + "M";
        }
        if (abs >= 1000) {
            String s = String.format(java.util.Locale.US, "%.1f", n / 1000);
            if (s.endsWith(".0")) {
                s = s.substring(0, s.length() - 2);
            }
            return s + "k";
        }
        if (Math.abs(n - Math.round(n)) < 1e-6) {
            return String.valueOf(Math.round(n));
        }
        return String.format(java.util.Locale.US, abs >= 10 ? "%.1f" : "%.2f", n);
    }
}
