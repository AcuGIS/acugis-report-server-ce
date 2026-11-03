# Download DuckDB JDBC (platform-specific) into stand-alone-tomcat-10/lib and
# docker/report-server for Tomcat shared classpath. Mirrors Designer sync-libs.ps1.
#
# Usage:
#   powershell -File stand-alone-tomcat-10/sync-duckdb-jdbc.ps1
#   powershell -File stand-alone-tomcat-10/sync-duckdb-jdbc.ps1 -Platform linux_amd64
param(
    [ValidateSet('windows_amd64', 'linux_amd64', 'osx_amd64', 'osx_arm64')]
    [string]$Platform = $(if ($env:OS -match 'Windows') { 'windows_amd64' } else { 'linux_amd64' }),
    [string]$Version = '1.5.5.1'
)

$ErrorActionPreference = 'Stop'
$root = Split-Path -Parent $PSScriptRoot
$libDirs = @(
    (Join-Path $PSScriptRoot 'lib'),
    (Join-Path $root 'docker\report-server')
)

$jarName = "duckdb_jdbc-$Version-$Platform.jar"
$url = "https://repo1.maven.org/maven2/org/duckdb/duckdb_jdbc/$Version/$jarName"

foreach ($dir in $libDirs) {
    if (-not (Test-Path $dir)) {
        New-Item -ItemType Directory -Force -Path $dir | Out-Null
    }
    $dest = Join-Path $dir $jarName
    if (-not (Test-Path $dest)) {
        Write-Host "GET $url"
        Invoke-WebRequest -Uri $url -OutFile $dest -UseBasicParsing
    } else {
        Write-Host "Exists: $dest"
    }
    Write-Host ("DuckDB JDBC: {0} ({1:N2} MB)" -f $dest, ((Get-Item $dest).Length / 1MB))
}

Write-Host ""
Write-Host "Install into Tomcat shared lib (required for DuckDB report fill):"
Write-Host "  copy $jarName to `$CATALINA_HOME/lib/ (or `$CATALINA_BASE/lib/)"
Write-Host "  Restart Tomcat after copying."
Write-Host ""
Write-Host "Docker compose: mount docker/report-server/$jarName into /usr/local/tomcat/lib/"
