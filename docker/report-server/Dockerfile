# AcuGIS Report Server — official Docker image (packaging only)
# Builds existing jrs-server and the Tomcat 10 Jakarta WAR from stand-alone-tomcat-10.
# Does not modify application source.
#
# Lives under docker/report-server/ as the official Compose image.
# (Legacy Tomcat 9 image definition at this path has been replaced.)

ARG GO_VERSION=1.26.3
ARG TOMCAT_VERSION=10.1.57
ARG JAVA_VERSION=17
# Open Source build (-tags oss).
ARG BUILD_EDITION=oss
ARG VERSION=1.0.0

# -----------------------------------------------------------------------------
# Stage 1: build existing jrs-server (CGO required for duckdb-go)
# -----------------------------------------------------------------------------
FROM golang:${GO_VERSION}-bookworm AS go-builder

RUN apt-get update \
    && apt-get install -y --no-install-recommends gcc g++ libc6-dev \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /src
COPY go-report-server/go.mod go-report-server/go.sum ./
RUN go mod download

COPY go-report-server/ ./
ENV CGO_ENABLED=1
ARG BUILD_EDITION=oss
ARG VERSION=1.0.0
ARG ACUGIS_LICENSE_PUBLIC_KEY_HEX=
# Bust the compile layer when source changes but VERSION stays 1.0.0.
# Passed by the Make targets. Empty or unknown is a failed build, not a stamp.
ARG SOURCE_REVISION
RUN set -eux; \
    printf '%s\n' "${SOURCE_REVISION}" | grep -Eq '^[0-9a-fA-F]{40}(-dirty)?$'; \
    echo "source-revision=${SOURCE_REVISION}"; \
    tags="-tags oss"; \
    ldflags="-X github.com/acugis/acugis-report-server-go/internal/config.DefaultAppVersion=${VERSION}"; \
    if [ -n "${ACUGIS_LICENSE_PUBLIC_KEY_HEX}" ]; then ldflags="$ldflags -X github.com/acugis/acugis-report-server-go/internal/license.productionPublicKeyHex=${ACUGIS_LICENSE_PUBLIC_KEY_HEX}"; fi; \
    modfile="$(go run ./scripts/saml-nofuzz-modfile.go)"; \
    modargs=""; \
    if [ -n "$modfile" ]; then modargs="-modfile $modfile -mod=readonly"; fi; \
    go build $modargs $tags -ldflags "$ldflags" -o /out/jrs-server ./cmd/jrs-server; \
    go build $modargs $tags -ldflags "$ldflags" -o /out/jrs-run-schedule ./cmd/jrs-run-schedule; \
    go build $modargs $tags -ldflags "$ldflags" -o /out/jrs-schedule-dispatcher ./cmd/jrs-schedule-dispatcher

# -----------------------------------------------------------------------------
# Stage 2: Jakarta WAR from source (stand-alone-tomcat-10/build.sh)
# -----------------------------------------------------------------------------
FROM maven:3.9.9-eclipse-temurin-17 AS war-builder
WORKDIR /src
COPY stand-alone-tomcat-10/pom.xml stand-alone-tomcat-10/pom.xml
COPY stand-alone-tomcat-10/src stand-alone-tomcat-10/src
COPY stand-alone-tomcat-10/map-src stand-alone-tomcat-10/map-src
COPY stand-alone-tomcat-10/sample-acugis.properties stand-alone-tomcat-10/sample-acugis.properties
COPY stand-alone-tomcat-10/dist-README.txt stand-alone-tomcat-10/dist-README.txt
WORKDIR /src/stand-alone-tomcat-10
RUN mvn -B -Pstandalone -DskipTests package

# -----------------------------------------------------------------------------
# Stage 3: runtime = JDK + Tomcat 10 + WAR + JDBC + Go + PHP seed tooling
# -----------------------------------------------------------------------------
ARG JAVA_VERSION=17
FROM eclipse-temurin:${JAVA_VERSION}-jdk-jammy

ARG TOMCAT_VERSION=10.1.57
ARG BUILD_EDITION=oss
ARG VERSION=1.0.0
ARG SOURCE_REVISION
ENV ACUGIS_BUILD_EDITION=${BUILD_EDITION}
LABEL org.opencontainers.image.revision="${SOURCE_REVISION}" \
      org.opencontainers.image.version="${VERSION}"

ENV CATALINA_HOME=/usr/local/tomcat \
    CATALINA_BASE=/usr/local/tomcat \
    JRS_JASPER_HOME=/opt/acugis-report-server \
    ACUGIS_DOCKER_IMAGE_ROOT=/opt/acugis-image \
    LANG=C.UTF-8 \
    LC_ALL=C.UTF-8

RUN apt-get update \
    && apt-get install -y --no-install-recommends \
        bash \
        ca-certificates \
        curl \
        fontconfig \
        fonts-dejavu-core \
        php-cli \
        php-pgsql \
        postgresql-client \
        tini \
        tzdata \
    && rm -rf /var/lib/apt/lists/*

# Tomcat 10.x (pinned)
RUN set -eux; \
    curl -fsSL "https://dlcdn.apache.org/tomcat/tomcat-10/v${TOMCAT_VERSION}/bin/apache-tomcat-${TOMCAT_VERSION}.tar.gz" -o /tmp/tomcat.tar.gz \
      || curl -fsSL "https://archive.apache.org/dist/tomcat/tomcat-10/v${TOMCAT_VERSION}/bin/apache-tomcat-${TOMCAT_VERSION}.tar.gz" -o /tmp/tomcat.tar.gz; \
    tar -xzf /tmp/tomcat.tar.gz -C /usr/local; \
    mv "/usr/local/apache-tomcat-${TOMCAT_VERSION}" /usr/local/tomcat; \
    rm -f /tmp/tomcat.tar.gz; \
    rm -rf /usr/local/tomcat/webapps/*; \
    mkdir -p \
      /usr/local/tomcat/acugis/reports \
      /usr/local/tomcat/acugis/uploads \
      /usr/local/tomcat/acugis/fonts \
      /usr/local/tomcat/acugis/drivers \
      /usr/local/tomcat/acugis/licenses \
      /usr/local/tomcat/acugis/temp \
      /usr/local/tomcat/acugis/resource-paths \
      /var/acugis/tomcat-conf \
      /data

# Existing Jakarta WAR as ROOT so /report and /acugis_report_image match Go defaults.
COPY --from=war-builder /src/stand-alone-tomcat-10/target/acugis-report-server.war /usr/local/tomcat/webapps/ROOT.war

# JDBC drivers (Tomcat shared lib). Linux DuckDB only in Linux images.
# IBM Db2 is not copied. Customers supply that jar; the entrypoint installs it.
COPY stand-alone-tomcat-10/lib/postgresql-42.7.3.jar /usr/local/tomcat/lib/
COPY stand-alone-tomcat-10/lib/mysql-connector-j-9.7.0.jar /usr/local/tomcat/lib/
COPY stand-alone-tomcat-10/lib/mssql-jdbc.jar /usr/local/tomcat/lib/
COPY stand-alone-tomcat-10/lib/mssql-jdbc-12.8.1-LICENSE.txt /usr/local/tomcat/lib/
COPY stand-alone-tomcat-10/lib/sqlite-jdbc.jar /usr/local/tomcat/lib/
COPY stand-alone-tomcat-10/lib/snowflake-jdbc.jar /usr/local/tomcat/lib/
COPY stand-alone-tomcat-10/lib/snowflake-jdbc-3.22.0-LICENSE.txt /usr/local/tomcat/lib/
COPY stand-alone-tomcat-10/lib/JDBC-DRIVER-LICENSES.txt /usr/local/tomcat/lib/
COPY stand-alone-tomcat-10/lib/duckdb_jdbc-1.5.5.1-linux_amd64.jar /usr/local/tomcat/lib/

# Sample Tomcat AcuGIS properties (standalone sample, unchanged content).
COPY stand-alone-tomcat-10/sample-acugis.properties /usr/local/tomcat/conf/acugis.properties

# Go binaries
COPY --from=go-builder /out/jrs-server /usr/local/bin/jrs-server
COPY --from=go-builder /out/jrs-run-schedule /usr/local/bin/jrs-run-schedule
COPY --from=go-builder /out/jrs-schedule-dispatcher /usr/local/bin/jrs-schedule-dispatcher

# Immutable image seed (copied into the report-resources volume on first boot).
WORKDIR /opt/acugis-image
COPY config.php ./
# Fail the image build if a database password is baked into config.php.
# JRS_DB_PASS is not set during docker build, so the fallback must be empty.
RUN php -r '$$c = require "/opt/acugis-image/config.php"; if (($$c["db"]["password"] ?? "") !== "") { fwrite(STDERR, "config.php contains a database password; credentials must come from the environment\n"); exit(1); }'
COPY lib ./lib
COPY scripts/seed_admin.php ./scripts/seed_admin.php
COPY database ./database
COPY docker/fixtures/app-root/ ./
COPY docker/fixtures/sql ./docker-fixtures/sql
COPY docker/fixtures/jrxml ./docker-fixtures/jrxml
COPY docker/report-server/context.xml.default ./docker-fixtures/context.xml.default
# Go UI reuses PHP-tree static assets (catalog CSS + federation shell + map JS).
COPY reports/portal.css ./reports/portal.css
COPY shared/ui ./shared/ui
COPY assets/js ./assets/js

RUN mkdir -p /opt/acugis-report-server \
    && if [ "$BUILD_EDITION" = "oss" ]; then \
         rm -f /opt/acugis-image/database/migrate_qgis.sql \
               /opt/acugis-image/database/migrate_arcgis.sql \
               /opt/acugis-image/assets/js/spatial-selection.js; \
         rm -rf /opt/acugis-image/database/migrations/professional; \
         rm -f /opt/acugis-image/database/migrations/009_dataset_intelligence.sql \
               /opt/acugis-image/database/migrations/010_audit_log.sql \
               /opt/acugis-image/database/migrations/011_oidc_external_identity.sql \
               /opt/acugis-image/database/migrations/012_saml_sso.sql \
               /opt/acugis-image/database/migrations/013_usage_analytics_indexes.sql \
               /opt/acugis-image/database/migrations/014_governance_metadata.sql \
               /opt/acugis-image/database/migrations/015_audit_export.sql; \
         if [ -f /opt/acugis-image/database/schema.oss.sql ]; then \
           cp /opt/acugis-image/database/schema.oss.sql /opt/acugis-image/database/schema.sql; \
         fi; \
       fi

COPY docker/report-server/docker-entrypoint.sh /usr/local/bin/docker-entrypoint.sh
RUN sed -i 's/\r$//' /usr/local/bin/docker-entrypoint.sh \
    && chmod +x /usr/local/bin/docker-entrypoint.sh /usr/local/bin/jrs-server /usr/local/bin/jrs-run-schedule /usr/local/bin/jrs-schedule-dispatcher

EXPOSE 9088

WORKDIR /opt/acugis-report-server
ENTRYPOINT ["/usr/bin/tini", "--", "/usr/local/bin/docker-entrypoint.sh"]
CMD ["jrs-server"]
