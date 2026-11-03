package com.acugis.reports;

import java.util.Collections;
import java.util.List;

import net.sf.jasperreports.charts.ChartTheme;
import net.sf.jasperreports.charts.ChartThemeBundle;
import net.sf.jasperreports.extensions.ExtensionsRegistry;
import net.sf.jasperreports.extensions.ExtensionsRegistryFactory;
import net.sf.jasperreports.engine.JRPropertiesMap;

/**
 * Registers the AcuGIS timing chart theme under {@link TimingChartTheme#THEME_NAME}.
 */
public final class AcugisChartsExtensionsRegistryFactory implements ExtensionsRegistryFactory {

    public static final ChartThemeBundle BUNDLE = new ChartThemeBundle() {
        @Override
        public String[] getChartThemeNames() {
            return new String[] {TimingChartTheme.THEME_NAME};
        }

        @Override
        public ChartTheme getChartTheme(String themeName) {
            if (TimingChartTheme.THEME_NAME.equals(themeName)) {
                return new TimingChartTheme();
            }
            return null;
        }
    };

    private static final ExtensionsRegistry REGISTRY = new ExtensionsRegistry() {
        @Override
        @SuppressWarnings("unchecked")
        public <T> List<T> getExtensions(Class<T> extensionType) {
            if (extensionType != null && ChartThemeBundle.class.isAssignableFrom(extensionType)) {
                return (List<T>) Collections.singletonList(BUNDLE);
            }
            return Collections.emptyList();
        }
    };

    @Override
    public ExtensionsRegistry createRegistry(String registryId, JRPropertiesMap properties) {
        return REGISTRY;
    }
}
