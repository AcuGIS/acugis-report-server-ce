package com.acugis.map;

import java.awt.BasicStroke;
import java.awt.Color;
import java.awt.Stroke;

/**
 * Hardcoded Phase-1 map styling. Future versions will replace this with
 * configurable / thematic style models.
 */
final class MapStyle {

    static final Color BACKGROUND = Color.WHITE;

    static final Color POLYGON_FILL = new Color(0xD3, 0xD3, 0xD3); // light gray
    static final Color POLYGON_OUTLINE = new Color(0xA9, 0xA9, 0xA9); // dark gray
    static final Stroke POLYGON_STROKE = new BasicStroke(1.0f);

    static final Color LINE_COLOR = new Color(0x00, 0x00, 0xFF); // blue
    static final Stroke LINE_STROKE = new BasicStroke(2.0f);

    static final Color POINT_COLOR = new Color(0xFF, 0x00, 0x00); // red
    static final int POINT_DIAMETER = 6;

    private MapStyle() {}
}
