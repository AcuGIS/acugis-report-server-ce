package com.acugis.reports;

import java.io.StringWriter;
import java.util.ArrayList;
import java.util.List;

import jakarta.servlet.http.HttpServletRequest;
import net.sf.jasperreports.charts.JRChart;
import net.sf.jasperreports.engine.JRElement;
import net.sf.jasperreports.engine.JRImage;
import net.sf.jasperreports.engine.JRElementGroup;
import net.sf.jasperreports.engine.JRException;
import net.sf.jasperreports.engine.JRGroup;
import net.sf.jasperreports.engine.JRPrintElement;
import net.sf.jasperreports.engine.JRPrintFrame;
import net.sf.jasperreports.engine.JRPrintImage;
import net.sf.jasperreports.engine.JRPrintPage;
import net.sf.jasperreports.engine.JRPrintText;
import net.sf.jasperreports.engine.JRSection;
import net.sf.jasperreports.engine.JRTextField;
import net.sf.jasperreports.engine.JasperPrint;
import net.sf.jasperreports.engine.JasperReport;
import net.sf.jasperreports.engine.base.JRBasePrintPage;

/**
 * Exports one filled report component after the normal report fill.
 *
 * <p>Tomcat activates this only when the Go embed-component route sets
 * {@link #HEADER} to {@link #HEADER_VALUE}. {@code /reports/execute} can carry
 * {@link #PARAMETER} and still takes the ordinary full-report export, because
 * that header is not a client-forwarded header and Go does not set it there.
 *
 * <p>Lookup uses {@link JRPrintElement#getKey()} on every page, including
 * elements nested in {@link JRPrintFrame}s. Design existence and type come
 * from the compiled report already loaded for the fill. No second compile or
 * fill is performed.
 *
 * <p>{@link #CODE_UNSUPPORTED} uses HTTP 422. The Embed ID is well formed and
 * a design element exists, but it is not a {@code textField}, a {@code chart},
 * or a Jasper {@code image} marked {@code acugis.component.type=map}. An
 * ordinary image or logo stays unsupported. 400 stays reserved for a malformed
 * Embed ID, which Go rejects before Jasper runs. 404 is not found or not
 * printed. 409 is more than one printed match.
 */
public final class EmbedComponentExporter {

    /** Must match go-report-server jasper.EmbedComponentHeader. */
    public static final String HEADER = "X-Acugis-Embed-Component";
    /** Must match go-report-server jasper.EmbedComponentHeaderValue. */
    public static final String HEADER_VALUE = "1";
    public static final String PARAMETER = "_componentId";

    public static final String CODE_NOT_FOUND = "COMPONENT_NOT_FOUND";
    public static final String CODE_NOT_PRINTED = "COMPONENT_NOT_PRINTED";
    public static final String CODE_AMBIGUOUS = "COMPONENT_AMBIGUOUS";
    public static final String CODE_UNSUPPORTED = "COMPONENT_UNSUPPORTED";
    public static final String CODE_SERVER_ERROR = "SERVER_ERROR";

    public static final int STATUS_NOT_FOUND = 404;
    public static final int STATUS_AMBIGUOUS = 409;
    /** Well-formed key whose design type cannot be component-embedded. */
    public static final int STATUS_UNSUPPORTED = 422;

    /**
     * Design-level classification for an AcuGIS Map element. The value is
     * written only by map JRXML. A filled map is a {@link JRPrintImage}, which
     * is also what a chart or an ordinary image becomes, so the print type
     * alone is not a map.
     */
    static final String MAP_TYPE_PROPERTY = "acugis.component.type";
    static final String MAP_TYPE_VALUE = "map";

    private EmbedComponentExporter() {
    }

    public static boolean componentMode(HttpServletRequest request) {
        return request != null && componentMode(request.getHeader(HEADER));
    }

    static boolean componentMode(String header) {
        return HEADER_VALUE.equals(header);
    }

    /**
     * The component id to export, or null when the trusted header is absent
     * or {@link #PARAMETER} is missing. The parameter value is not trimmed or
     * lowercased.
     */
    public static String trustedComponentId(HttpServletRequest request) {
        if (request == null) {
            return null;
        }
        return trustedComponentId(request.getHeader(HEADER), request.getParameter(PARAMETER));
    }

    static String trustedComponentId(String header, String parameter) {
        if (!componentMode(header) || parameter == null || parameter.isEmpty()) {
            return null;
        }
        return parameter;
    }

    /**
     * Select the single supported print element and export it with the same
     * HTML exporter the full report uses. The element's position is restored
     * after export so the filled {@link JasperPrint} stays intact.
     */
    public static String exportHtml(
            JasperReport report,
            JasperPrint filled,
            String embedId,
            HttpServletRequest request) throws ComponentExportException, JRException {
        JRPrintElement element = select(report, filled, embedId);
        return renderSlice(filled, element, request);
    }

    static JRPrintElement select(JasperReport report, JasperPrint filled, String embedId)
            throws ComponentExportException {
        if (report == null || filled == null || embedId == null || embedId.isEmpty()) {
            throw failed(500, CODE_SERVER_ERROR, "component export failed");
        }
        List<JRElement> design = designElements(report, embedId);
        if (design.isEmpty()) {
            throw failed(STATUS_NOT_FOUND, CODE_NOT_FOUND, "no design element with this key");
        }
        for (JRElement element : design) {
            if (!supportedDesign(element)) {
                throw failed(STATUS_UNSUPPORTED, CODE_UNSUPPORTED, "design element is not a text field, chart, or map");
            }
        }
        List<JRPrintElement> printed = printElements(filled, embedId);
        if (printed.isEmpty()) {
            throw failed(STATUS_NOT_FOUND, CODE_NOT_PRINTED, "design element did not print");
        }
        if (printed.size() != 1) {
            throw failed(STATUS_AMBIGUOUS, CODE_AMBIGUOUS, "more than one printed element has this key");
        }
        JRPrintElement one = printed.get(0);
        if (!printMatchesDesign(design, one)) {
            throw failed(STATUS_UNSUPPORTED, CODE_UNSUPPORTED, "printed element is not a text field, chart, or map");
        }
        return one;
    }

    static List<JRElement> designElements(JasperReport report, String embedId) {
        List<JRElement> hits = new ArrayList<JRElement>();
        if (report == null || embedId == null) {
            return hits;
        }
        walkDesign(report.getBackground(), embedId, hits);
        walkDesign(report.getTitle(), embedId, hits);
        walkDesign(report.getPageHeader(), embedId, hits);
        walkDesign(report.getColumnHeader(), embedId, hits);
        walkSection(report.getDetailSection(), embedId, hits);
        walkDesign(report.getColumnFooter(), embedId, hits);
        walkDesign(report.getPageFooter(), embedId, hits);
        walkDesign(report.getLastPageFooter(), embedId, hits);
        walkDesign(report.getSummary(), embedId, hits);
        walkDesign(report.getNoData(), embedId, hits);
        JRGroup[] groups = report.getGroups();
        if (groups != null) {
            for (JRGroup group : groups) {
                if (group == null) {
                    continue;
                }
                walkSection(group.getGroupHeaderSection(), embedId, hits);
                walkSection(group.getGroupFooterSection(), embedId, hits);
            }
        }
        return hits;
    }

    static List<JRPrintElement> printElements(JasperPrint filled, String embedId) {
        List<JRPrintElement> hits = new ArrayList<JRPrintElement>();
        if (filled == null || embedId == null || filled.getPages() == null) {
            return hits;
        }
        for (JRPrintPage page : filled.getPages()) {
            if (page != null) {
                walkPrint(page.getElements(), embedId, hits);
            }
        }
        return hits;
    }

    private static boolean supportedDesign(JRElement element) {
        return element instanceof JRTextField || element instanceof JRChart || markedMapImage(element);
    }

    /**
     * A map is a design {@link JRImage} carrying {@link #MAP_TYPE_PROPERTY}.
     * Charts are classified as charts. Every other image is unsupported.
     */
    static boolean markedMapImage(JRElement element) {
        if (!(element instanceof JRImage) || element instanceof JRChart || element.getPropertiesMap() == null) {
            return false;
        }
        return MAP_TYPE_VALUE.equals(element.getPropertiesMap().getProperty(MAP_TYPE_PROPERTY));
    }

    private static boolean printMatchesDesign(List<JRElement> design, JRPrintElement printed) {
        if (printed instanceof JRPrintText) {
            for (JRElement element : design) {
                if (element instanceof JRTextField) {
                    return true;
                }
            }
            return false;
        }
        if (printed instanceof JRPrintImage) {
            for (JRElement element : design) {
                // The design element was already required to be a supported
                // chart or a marked map. The filled image is not reclassified
                // from its renderer, raster, or dimensions.
                if (element instanceof JRChart || markedMapImage(element)) {
                    return true;
                }
            }
            return false;
        }
        return false;
    }

    private static String renderSlice(JasperPrint source, JRPrintElement element, HttpServletRequest request)
            throws JRException {
        int x = element.getX();
        int y = element.getY();
        element.setX(0);
        element.setY(0);
        try {
            JasperPrint slice = new JasperPrint();
            slice.setName(source.getName());
            slice.setPageWidth(element.getWidth());
            slice.setPageHeight(element.getHeight());
            slice.setOrientation(source.getOrientation());
            slice.setDefaultStyle(source.getDefaultStyle());
            JRPrintPage page = new JRBasePrintPage();
            page.addElement(element);
            slice.addPage(page);

            StringWriter writer = new StringWriter();
            ReportService.writeHtml(slice, writer, request);
            return writer.toString();
        } finally {
            element.setX(x);
            element.setY(y);
        }
    }

    private static void walkSection(JRSection section, String embedId, List<JRElement> hits) {
        if (section == null || section.getBands() == null) {
            return;
        }
        for (int i = 0; i < section.getBands().length; i++) {
            walkDesign(section.getBands()[i], embedId, hits);
        }
    }

    private static void walkDesign(JRElementGroup group, String embedId, List<JRElement> hits) {
        if (group == null || group.getElements() == null) {
            return;
        }
        for (JRElement element : group.getElements()) {
            if (element == null) {
                continue;
            }
            if (embedId.equals(element.getKey())) {
                hits.add(element);
            }
            if (element instanceof JRElementGroup) {
                walkDesign((JRElementGroup) element, embedId, hits);
            }
        }
    }

    private static void walkPrint(List<JRPrintElement> elements, String embedId, List<JRPrintElement> hits) {
        if (elements == null) {
            return;
        }
        for (JRPrintElement element : elements) {
            if (element == null) {
                continue;
            }
            if (embedId.equals(element.getKey())) {
                hits.add(element);
            }
            if (element instanceof JRPrintFrame) {
                walkPrint(((JRPrintFrame) element).getElements(), embedId, hits);
            }
        }
    }

    private static ComponentExportException failed(int status, String code, String detail) {
        return new ComponentExportException(status, code, detail);
    }

    public static final class ComponentExportException extends Exception {
        private static final long serialVersionUID = 1L;
        private final int status;
        private final String code;

        ComponentExportException(int status, String code, String detail) {
            super(detail == null || detail.isEmpty() ? code : detail);
            this.status = status;
            this.code = code;
        }

        public int status() {
            return status;
        }

        public String code() {
            return code;
        }

        public String detail() {
            return getMessage();
        }
    }
}
