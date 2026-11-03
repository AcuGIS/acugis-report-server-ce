package com.acugis.reports;

import java.util.Map;
import net.sf.jasperreports.engine.DefaultJasperReportsContext;
import net.sf.jasperreports.engine.JRDataset;
import net.sf.jasperreports.engine.JRException;
import net.sf.jasperreports.engine.JRValueParameter;
import net.sf.jasperreports.engine.JasperReportsContext;
import net.sf.jasperreports.engine.query.JREmptyQueryExecuter;
import net.sf.jasperreports.engine.query.JRQueryExecuter;
import net.sf.jasperreports.engine.query.QueryExecuterFactory;

/**
 * Stub query executer factory for AcuGIS collection providers
 * (Cassandra / ArcGIS / BigQuery via language="collection").
 *
 * <p>Same registration Designer uses in {@code com.acugis.studio.CollectionQueryExecuterFactory}.
 * JRXML keeps {@code language="cassandra"} or {@code language="collection"} so Report Server
 * can read the query text. JasperReports 7 requires a factory registered on the
 * {@link JasperReportsContext} to compile that JRXML. Fill still uses
 * {@link net.sf.jasperreports.engine.data.JRMapCollectionDataSource} from
 * {@link CollectionFill}; this executer is not the data path.
 *
 * <p>DuckDB fills via JDBC {@code REPORT_CONNECTION} with {@code language="SQL"}.
 * BigQuery uses {@code language="collection"} with {@code acugis.datasource.provider=bigquery}.
 */
public final class CollectionQueryExecuterFactory implements QueryExecuterFactory {

    public static final String LANGUAGE_CASSANDRA = "cassandra";
    public static final String LANGUAGE_COLLECTION = "collection";

    /** Register collection languages for compile and for any dataset that resolves a query executer. */
    public static void register() {
        String factory = CollectionQueryExecuterFactory.class.getName();
        JasperReportsContext ctx = DefaultJasperReportsContext.getInstance();
        // JR 7 resolves factories via JasperReportsContext properties (not System props alone).
        ctx.setProperty(QUERY_EXECUTER_FACTORY_PREFIX + LANGUAGE_CASSANDRA, factory);
        ctx.setProperty(QUERY_EXECUTER_FACTORY_PREFIX + LANGUAGE_COLLECTION, factory);
        ctx.setProperty(QUERY_EXECUTER_FACTORY_PREFIX + "Cassandra", factory);
        ctx.setProperty(QUERY_EXECUTER_FACTORY_PREFIX + "COLLECTION", factory);
        System.setProperty(QUERY_EXECUTER_FACTORY_PREFIX + LANGUAGE_CASSANDRA, factory);
        System.setProperty(QUERY_EXECUTER_FACTORY_PREFIX + LANGUAGE_COLLECTION, factory);
    }

    @Override
    public Object[] getBuiltinParameters() {
        return new Object[] {};
    }

    @Override
    public JRQueryExecuter createQueryExecuter(
            JasperReportsContext jasperReportsContext,
            JRDataset dataset,
            Map<String, ? extends JRValueParameter> parameters)
            throws JRException {
        // Collection fill passes JRMapCollectionDataSource. This path is only
        // reached if a dataset fill resolves the report query language.
        return new JREmptyQueryExecuter();
    }

    @Override
    public boolean supportsQueryParameterType(String className) {
        return true;
    }
}
