package com.acugis.map;

import java.util.logging.Level;
import java.util.logging.Logger;

/** Timing + metadata for a single map element evaluation. */
final class MapPerfRecord {

    private static final Logger LOG = Logger.getLogger(MapPerfRecord.class.getName());

    String reportName = "";
    String mapUuid = "";
    int mapSequence;
    String groupValue = "";
    String datasourceType = "";
    String extentMode = "";
    int evaluationCount;
    boolean cacheHit;

    long jdbcAcquisitionMs;
    long sqlPrepareMs;
    long sqlExecutionMs;
    long geometryDecodeMs;
    long coordTransformMs;
    long extentMs;
    long basemapMs;
    long featureRenderMs;
    long labelRenderMs;
    long legendRenderMs;
    long pngEncodeMs;
    long totalMs;

    int featureCount;
    int invalidSkipped;
    int queryCount;
    String geometryType = "";
    int width;
    int height;
    int outputBytes;

    void log() {
        if (!MapPerfFlags.perfEnabled()) {
            return;
        }
        StringBuilder sb = new StringBuilder(384);
        sb.append("map_perf")
                .append(" report=").append(nullToEmpty(reportName))
                .append(" map_uuid=").append(nullToEmpty(mapUuid))
                .append(" map_sequence=").append(mapSequence)
                .append(" group=").append(nullToEmpty(groupValue))
                .append(" datasource_type=").append(nullToEmpty(datasourceType))
                .append(" extent_mode=").append(nullToEmpty(extentMode))
                .append(" evaluation_count=").append(evaluationCount)
                .append(" cache=").append(cacheHit ? "hit" : "miss")
                .append(" features=").append(featureCount)
                .append(" invalid_skipped=").append(invalidSkipped)
                .append(" geometry_type=").append(nullToEmpty(geometryType))
                .append(" width=").append(width).append(" height=").append(height)
                .append(" output_bytes=").append(outputBytes)
                .append(" jdbc_acquisition_ms=").append(jdbcAcquisitionMs)
                .append(" sql_preparation_ms=").append(sqlPrepareMs)
                .append(" sql_execution_ms=").append(sqlExecutionMs)
                .append(" geometry_decode_ms=").append(geometryDecodeMs)
                .append(" coord_transform_ms=").append(coordTransformMs)
                .append(" extent_ms=").append(extentMs)
                .append(" basemap_ms=").append(basemapMs)
                .append(" feature_render_ms=").append(featureRenderMs)
                .append(" label_render_ms=").append(labelRenderMs)
                .append(" legend_render_ms=").append(legendRenderMs)
                .append(" png_encode_ms=").append(pngEncodeMs)
                .append(" total_ms=").append(totalMs);
        LOG.log(Level.INFO, sb.toString());
    }

    private static String nullToEmpty(String s) {
        return s == null ? "" : s;
    }
}
