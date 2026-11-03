package com.acugis.reports;

import java.lang.reflect.InvocationHandler;
import java.lang.reflect.Method;
import java.lang.reflect.Proxy;
import java.sql.Connection;
import java.sql.PreparedStatement;
import java.sql.ResultSet;
import java.sql.Statement;
import java.util.ArrayList;
import java.util.Collections;
import java.util.List;
import java.util.concurrent.atomic.AtomicInteger;
import java.util.concurrent.atomic.AtomicLong;

/**
 * Per-request JDBC timing: SQL execution ms, query count, rows read, metadata ms.
 * Installed on the fill thread so Jasper's query executer is measured separately
 * from JasperFillManager CPU work.
 */
public final class SqlTimingContext {

    private static final ThreadLocal<SqlTimingContext> CURRENT = new ThreadLocal<>();

    private final AtomicLong sqlNanos = new AtomicLong();
    private final AtomicLong fetchNanos = new AtomicLong();
    private final AtomicLong metadataNanos = new AtomicLong();
    private final AtomicInteger queryCount = new AtomicInteger();
    private final AtomicLong rowCount = new AtomicLong();
    private final List<QuerySample> queries = Collections.synchronizedList(new ArrayList<QuerySample>());

    public static final class QuerySample {
        public final long executeMs;
        public final String sql;

        QuerySample(long executeMs, String sql) {
            this.executeMs = executeMs;
            this.sql = sql == null ? "" : sql;
        }
    }

    private SqlTimingContext() {
    }

    public static SqlTimingContext install() {
        SqlTimingContext ctx = new SqlTimingContext();
        CURRENT.set(ctx);
        return ctx;
    }

    public static SqlTimingContext current() {
        return CURRENT.get();
    }

    public static void clear() {
        CURRENT.remove();
    }

    public long sqlMs() {
        return sqlNanos.get() / 1_000_000L;
    }

    public long fetchMs() {
        return fetchNanos.get() / 1_000_000L;
    }

    public long metadataMs() {
        return metadataNanos.get() / 1_000_000L;
    }

    public int queries() {
        return queryCount.get();
    }

    public long rows() {
        return rowCount.get();
    }

    public List<QuerySample> querySamples() {
        synchronized (queries) {
            return new ArrayList<QuerySample>(queries);
        }
    }

    void addExecuteNanos(long nanos, String sql) {
        if (nanos > 0) {
            sqlNanos.addAndGet(nanos);
        }
        queryCount.incrementAndGet();
        long ms = nanos / 1_000_000L;
        String trimmed = sql == null ? "" : sql.replaceAll("\\s+", " ").trim();
        if (trimmed.length() > 180) {
            trimmed = trimmed.substring(0, 180) + "…";
        }
        queries.add(new QuerySample(ms, trimmed));
    }

    void addFetchNanos(long nanos) {
        if (nanos > 0) {
            fetchNanos.addAndGet(nanos);
        }
    }

    void addMetadataNanos(long nanos) {
        if (nanos > 0) {
            metadataNanos.addAndGet(nanos);
        }
    }

    void addRows(long n) {
        if (n > 0) {
            rowCount.addAndGet(n);
        }
    }

    /** Dynamic proxy around a pooled Connection. */
    public static Connection wrap(Connection connection) {
        if (connection == null) {
            return null;
        }
        return (Connection) Proxy.newProxyInstance(
                Connection.class.getClassLoader(),
                new Class<?>[] {Connection.class},
                new ConnectionHandler(connection));
    }

    private static Object invokeDelegate(Object delegate, Method method, Object[] args) throws Throwable {
        try {
            return method.invoke(delegate, args);
        } catch (java.lang.reflect.InvocationTargetException e) {
            Throwable cause = e.getCause();
            throw cause == null ? e : cause;
        }
    }

    private static final class ConnectionHandler implements InvocationHandler {
        private final Connection delegate;

        ConnectionHandler(Connection delegate) {
            this.delegate = delegate;
        }

        @Override
        public Object invoke(Object proxy, Method method, Object[] args) throws Throwable {
            String name = method.getName();
            if ("unwrap".equals(name) && args != null && args.length == 1 && args[0] instanceof Class) {
                Class<?> iface = (Class<?>) args[0];
                if (iface.isInstance(proxy)) {
                    return proxy;
                }
                return delegate.unwrap(iface);
            }
            if ("isWrapperFor".equals(name) && args != null && args.length == 1 && args[0] instanceof Class) {
                Class<?> iface = (Class<?>) args[0];
                return iface.isInstance(proxy) || delegate.isWrapperFor(iface);
            }
            if ("getMetaData".equals(name)) {
                SqlTimingContext ctx = current();
                long start = System.nanoTime();
                try {
                    return invokeDelegate(delegate, method, args);
                } finally {
                    if (ctx != null) {
                        ctx.addMetadataNanos(System.nanoTime() - start);
                    }
                }
            }
            Object result = invokeDelegate(delegate, method, args);
            if (result instanceof PreparedStatement) {
                String sql = (args != null && args.length > 0 && args[0] instanceof String)
                        ? (String) args[0] : null;
                return wrapStatement((PreparedStatement) result, PreparedStatement.class, sql);
            }
            if (result instanceof Statement) {
                return wrapStatement((Statement) result, Statement.class, null);
            }
            return result;
        }
    }

    private static Object wrapStatement(Statement statement, Class<?> iface, String preparedSql) {
        return Proxy.newProxyInstance(
                iface.getClassLoader(),
                new Class<?>[] {iface},
                new StatementHandler(statement, preparedSql));
    }

    private static final class StatementHandler implements InvocationHandler {
        private final Statement delegate;
        private final String preparedSql;

        StatementHandler(Statement delegate, String preparedSql) {
            this.delegate = delegate;
            this.preparedSql = preparedSql;
        }

        @Override
        public Object invoke(Object proxy, Method method, Object[] args) throws Throwable {
            String name = method.getName();
            boolean timed = "executeQuery".equals(name) || "execute".equals(name) || "executeUpdate".equals(name);
            if ("getMetaData".equals(name)) {
                SqlTimingContext ctx = current();
                long start = System.nanoTime();
                try {
                    return invokeDelegate(delegate, method, args);
                } finally {
                    if (ctx != null) {
                        ctx.addMetadataNanos(System.nanoTime() - start);
                    }
                }
            }
            if (!timed) {
                Object result = invokeDelegate(delegate, method, args);
                if (result instanceof ResultSet) {
                    return wrapResultSet((ResultSet) result);
                }
                return result;
            }
            SqlTimingContext ctx = current();
            String sql = preparedSql;
            if (sql == null && args != null && args.length > 0 && args[0] instanceof String) {
                sql = (String) args[0];
            }
            long start = System.nanoTime();
            try {
                Object result = invokeDelegate(delegate, method, args);
                if (result instanceof ResultSet) {
                    return wrapResultSet((ResultSet) result);
                }
                return result;
            } finally {
                if (ctx != null) {
                    ctx.addExecuteNanos(System.nanoTime() - start, sql);
                }
            }
        }
    }

    private static ResultSet wrapResultSet(ResultSet rs) {
        return (ResultSet) Proxy.newProxyInstance(
                ResultSet.class.getClassLoader(),
                new Class<?>[] {ResultSet.class},
                new ResultSetHandler(rs));
    }

    private static final class ResultSetHandler implements InvocationHandler {
        private final ResultSet delegate;
        private long rows;

        ResultSetHandler(ResultSet delegate) {
            this.delegate = delegate;
        }

        @Override
        public Object invoke(Object proxy, Method method, Object[] args) throws Throwable {
            String name = method.getName();
            if ("next".equals(name)) {
                long start = System.nanoTime();
                Object ok = invokeDelegate(delegate, method, args);
                SqlTimingContext ctx = current();
                if (ctx != null) {
                    ctx.addFetchNanos(System.nanoTime() - start);
                    if (Boolean.TRUE.equals(ok)) {
                        rows++;
                    } else {
                        flushRows();
                    }
                } else if (Boolean.TRUE.equals(ok)) {
                    rows++;
                } else {
                    flushRows();
                }
                return ok;
            }
            if ("close".equals(name)) {
                flushRows();
                return invokeDelegate(delegate, method, args);
            }
            return invokeDelegate(delegate, method, args);
        }

        private void flushRows() {
            SqlTimingContext ctx = current();
            if (ctx != null && rows > 0) {
                ctx.addRows(rows);
                rows = 0;
            }
        }
    }
}
