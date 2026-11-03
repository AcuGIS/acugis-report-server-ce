package com.acugis.reports;

public class FormatUtil {
    public static String normalizeFormat(String format) {
        if (format == null) {
            return null;
        }
        String upper = format.toUpperCase().trim();
        if ("EXCEL".equals(upper)) {
            return "XLSX";
        }
        if ("WORD".equals(upper)) {
            return "DOCX";
        }
        return upper;
    }

    public static String getContentType(String format) {
        if (format == null) {
            return "application/octet-stream";
        }
        switch (format) {
            case "PDF": {
                return "application/pdf";
            }
            case "HTML": {
                return "text/html; charset=UTF-8";
            }
            case "XLSX": {
                return "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet";
            }
            case "DOCX": {
                return "application/vnd.openxmlformats-officedocument.wordprocessingml.document";
            }
        }
        return "application/octet-stream";
    }

    public static String getExtension(String format) {
        if (format == null) {
            return "";
        }
        switch (format) {
            case "PDF": {
                return ".pdf";
            }
            case "HTML": {
                return ".html";
            }
            case "XLSX": {
                return ".xlsx";
            }
            case "DOCX": {
                return ".docx";
            }
        }
        return "";
    }

    public static boolean isSupported(String format) {
        if (format == null) {
            return false;
        }
        return "PDF".equals(format) || "HTML".equals(format) || "XLSX".equals(format) || "DOCX".equals(format);
    }
}
