AcuGIS Report Server — Standalone Tomcat 10 WAR
================================================

Contents
--------
  acugis-report-server.war     Application WAR (Jakarta EE / Tomcat 10.x)
  sample-acugis.properties     Sample external config for CATALINA_BASE/conf/
  README.txt                   This file

Quick start
-----------
1. Install Apache Tomcat 10.x (JDK 11+).
2. Copy acugis-report-server.war to CATALINA_BASE/webapps/
   (or rename to ROOT.war for context path "/").
3. Create directories under CATALINA_BASE/acugis/:
     reports  uploads  fonts  drivers  licenses  temp
4. Copy sample-acugis.properties to CATALINA_BASE/conf/acugis.properties
   and adjust paths if needed.
5. Install JDBC drivers into CATALINA_HOME/lib/ (or CATALINA_BASE/lib/).
   Driver jars are not bundled in the WAR — see stand-alone-tomcat-10/lib/ for copies.
6. Configure JNDI datasources in CATALINA_BASE/conf/context.xml
   (see ../docker/report-server/context.xml.example for examples).
7. Place JRXML / .jasper files in CATALINA_BASE/acugis/reports/
8. Start Tomcat and check catalina logs for the AcuGIS startup summary.

Endpoints (prefix with context path)
------------------------------------
  /report                 Execute report
  /acugis_report_image    HTML export images
  /testConnection         JDBC test helper
  /pingSession            Session diagnostics
  /image                  Jasper ImageServlet

Environment overrides (examples)
--------------------------------
  ACUGIS_REPORTS_DIR, ACUGIS_UPLOADS_DIR, ACUGIS_FONTS_DIR,
  ACUGIS_DRIVERS_DIR, ACUGIS_LICENSES_DIR, ACUGIS_LICENSE_FILE,
  REPORTS_DIR (legacy alias for reports.dir)

Full guide: see INSTALL.md next to this dist folder (or in the stand-alone-tomcat-10 source tree).

Docker / Tomcat 9
-----------------
The existing Docker build under docker/report-server is unchanged.
This WAR is a separate standalone Tomcat 10 deployment option.
