package com.acugis.map;

import java.awt.Color;
import java.awt.Font;
import java.awt.FontMetrics;
import java.awt.Graphics2D;
import java.awt.RenderingHints;
import java.awt.image.BufferedImage;
import java.util.ArrayList;
import java.util.List;

/**
 * Placeholder image shown when map rendering fails.
 * Keeps report fill from aborting on map errors.
 */
final class ErrorImage {

    private ErrorImage() {}

    static BufferedImage create(int width, int height, Throwable error) {
        int w = Math.max(1, width);
        int h = Math.max(1, height);
        BufferedImage image = new BufferedImage(w, h, BufferedImage.TYPE_INT_RGB);
        Graphics2D g = image.createGraphics();
        try {
            g.setRenderingHint(RenderingHints.KEY_ANTIALIASING, RenderingHints.VALUE_ANTIALIAS_ON);
            g.setRenderingHint(RenderingHints.KEY_TEXT_ANTIALIASING, RenderingHints.VALUE_TEXT_ANTIALIAS_ON);
            g.setColor(new Color(0xFA, 0xFA, 0xFA));
            g.fillRect(0, 0, w, h);
            g.setColor(new Color(0xCC, 0x66, 0x66));
            g.drawRect(1, 1, w - 3, h - 3);

            String message = error == null ? "Unknown error" : String.valueOf(error.getMessage());
            if (message == null || message.trim().isEmpty()) {
                message = error.getClass().getSimpleName();
            }

            List<String> lines = new ArrayList<String>();
            lines.add("Unable to Render Map");
            lines.add("");
            wrap(message.trim(), 48, lines);

            g.setFont(new Font(Font.SANS_SERIF, Font.BOLD, Math.max(10, Math.min(14, w / 28))));
            FontMetrics fm = g.getFontMetrics();
            int lineHeight = fm.getHeight();
            int totalHeight = lines.size() * lineHeight;
            int y = Math.max(fm.getAscent() + 8, (h - totalHeight) / 2 + fm.getAscent());

            g.setColor(new Color(0x66, 0x22, 0x22));
            for (int i = 0; i < lines.size(); i++) {
                String line = lines.get(i);
                if (i == 0) {
                    g.setFont(new Font(Font.SANS_SERIF, Font.BOLD, Math.max(11, Math.min(16, w / 24))));
                    fm = g.getFontMetrics();
                } else if (i == 1) {
                    continue;
                } else {
                    g.setFont(new Font(Font.SANS_SERIF, Font.PLAIN, Math.max(9, Math.min(12, w / 32))));
                    fm = g.getFontMetrics();
                    lineHeight = fm.getHeight();
                }
                int x = Math.max(8, (w - fm.stringWidth(line)) / 2);
                g.drawString(line, x, y);
                y += lineHeight + (i == 0 ? 6 : 0);
            }
        } finally {
            g.dispose();
        }
        return image;
    }

    private static void wrap(String text, int maxChars, List<String> out) {
        if (text.length() <= maxChars) {
            out.add(text);
            return;
        }
        int start = 0;
        while (start < text.length()) {
            int end = Math.min(text.length(), start + maxChars);
            if (end < text.length()) {
                int space = text.lastIndexOf(' ', end);
                if (space > start + maxChars / 2) {
                    end = space;
                }
            }
            out.add(text.substring(start, end).trim());
            start = end;
            while (start < text.length() && text.charAt(start) == ' ') {
                start++;
            }
        }
    }
}
