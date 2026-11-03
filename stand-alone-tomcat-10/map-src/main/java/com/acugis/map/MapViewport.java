package com.acugis.map;

import java.util.List;
import org.locationtech.jts.geom.Coordinate;
import org.locationtech.jts.geom.Envelope;
import org.locationtech.jts.geom.Geometry;

/**
 * World-to-pixel transform with aspect-preserving auto-scale and padding.
 * Extensible for future north-arrows / scale-bars that need scale metadata.
 */
final class MapViewport {

    final int width;
    final int height;
    final Envelope world;
    /** Uniform scale used by feature painting (aspect-preserving path). */
    final double scale;
    final double scaleX;
    final double scaleY;
    final double offsetX;
    final double offsetY;

    private MapViewport(
            int width,
            int height,
            Envelope world,
            double scaleX,
            double scaleY,
            double offsetX,
            double offsetY) {
        this.width = width;
        this.height = height;
        this.world = world;
        this.scaleX = scaleX;
        this.scaleY = scaleY;
        this.scale = Math.min(scaleX, scaleY);
        this.offsetX = offsetX;
        this.offsetY = offsetY;
    }

    /** Backward-compatible constructor for uniform scale. */
    private MapViewport(int width, int height, Envelope world, double scale, double offsetX, double offsetY) {
        this(width, height, world, scale, scale, offsetX, offsetY);
    }

    static Envelope envelopeOf(List<Geometry> geometries) {
        Envelope env = new Envelope();
        if (geometries != null) {
            for (Geometry g : geometries) {
                if (g != null && !g.isEmpty()) {
                    env.expandToInclude(g.getEnvelopeInternal());
                }
            }
        }
        return env;
    }

    static MapViewport fromEnvelope(Envelope envelope, int width, int height, double paddingFraction) {
        Envelope world = envelope == null ? new Envelope() : new Envelope(envelope);
        if (world.isNull()) {
            world = new Envelope(-1, 1, -1, 1);
        }

        double w = world.getWidth();
        double h = world.getHeight();
        if (w <= 0) {
            double pad = Math.max(Math.abs(world.getMinX()) * 0.01, 1.0);
            world.expandBy(pad, 0);
            w = world.getWidth();
        }
        if (h <= 0) {
            double pad = Math.max(Math.abs(world.getMinY()) * 0.01, 1.0);
            world.expandBy(0, pad);
            h = world.getHeight();
        }

        double pad = Math.max(0.0, paddingFraction);
        world.expandBy(world.getWidth() * pad, world.getHeight() * pad);
        w = world.getWidth();
        h = world.getHeight();

        double scaleX = width / w;
        double scaleY = height / h;
        double scale = Math.min(scaleX, scaleY);

        double drawW = w * scale;
        double drawH = h * scale;
        double offsetX = (width - drawW) / 2.0 - world.getMinX() * scale;
        // Screen Y grows downward; flip world Y.
        double offsetY = (height - drawH) / 2.0 + world.getMaxY() * scale;

        return new MapViewport(width, height, world, scale, offsetX, offsetY);
    }

    /**
     * Stretch world envelope to fill the image with independent X/Y scales
     * (WMS GetMap behaviour). Used when compositing basemap under GeoServer GetMap.
     */
    static MapViewport stretchFrame(Envelope envelope, int width, int height) {
        Envelope world = envelope == null ? new Envelope() : new Envelope(envelope);
        if (world.isNull()) {
            world = new Envelope(-1, 1, -1, 1);
        }
        double w = world.getWidth();
        double h = world.getHeight();
        if (w <= 0) {
            world.expandBy(1, 0);
            w = world.getWidth();
        }
        if (h <= 0) {
            world.expandBy(0, 1);
            h = world.getHeight();
        }
        double scaleX = width / w;
        double scaleY = height / h;
        double offsetX = -world.getMinX() * scaleX;
        double offsetY = world.getMaxY() * scaleY;
        return new MapViewport(width, height, world, scaleX, scaleY, offsetX, offsetY);
    }

    Coordinate toPixel(Coordinate c) {
        double x = c.x * scaleX + offsetX;
        double y = -c.y * scaleY + offsetY;
        return new Coordinate(x, y);
    }
}
