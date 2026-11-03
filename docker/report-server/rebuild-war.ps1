# Rebuild acugis-report-server.war from src/ and acugis-report-server.war.bak.
# Uses forward-slash zip entry names (required by Tomcat on all platforms).
#
# Also packs MapRenderer jars from map-lib/ into WEB-INF/lib when that
# directory has already been populated. This script does not download
# or build those jars, and it does not look for a Designer checkout.
# Packs Barcode4J / QR (jasperreports-barcode4j) from barcode-lib/:
#   powershell -File sync-barcode-lib.ps1
param(
    [string]$BaseDir = $PSScriptRoot
)

$ErrorActionPreference = 'Stop'
$bak = Join-Path $BaseDir 'acugis-report-server.war.bak'
$war = Join-Path $BaseDir 'acugis-report-server.war'
$work = Join-Path $BaseDir '_war_rebuild'
$classesOut = Join-Path $BaseDir '_classes_out'
$ecj = Join-Path $BaseDir '_ecj.jar'
$servletApi = Join-Path $BaseDir '_servlet-api.jar'
$mapLib = Join-Path $BaseDir 'map-lib'
$barcodeLib = Join-Path $BaseDir 'barcode-lib'

if (-not (Test-Path $bak)) {
    throw "Missing backup WAR: $bak"
}

@($work, $classesOut) | ForEach-Object {
    if (Test-Path $_) { Remove-Item $_ -Recurse -Force }
    New-Item -ItemType Directory -Path $_ | Out-Null
}

if (-not (Test-Path $ecj)) {
    Invoke-WebRequest -Uri 'https://repo1.maven.org/maven2/org/eclipse/jdt/ecj/3.24.0/ecj-3.24.0.jar' -OutFile $ecj -UseBasicParsing
}
if (-not (Test-Path $servletApi)) {
    Invoke-WebRequest -Uri 'https://repo1.maven.org/maven2/javax/servlet/javax.servlet-api/4.0.1/javax.servlet-api-4.0.1.jar' -OutFile $servletApi -UseBasicParsing
}

Add-Type -AssemblyName System.IO.Compression
Add-Type -AssemblyName System.IO.Compression.FileSystem
[System.IO.Compression.ZipFile]::ExtractToDirectory($bak, $work)

$libPath = Join-Path $work 'WEB-INF\lib'
$warClasses = Join-Path $work 'WEB-INF\classes'
$cp = "$warClasses;$servletApi;" + ((Get-ChildItem $libPath -Filter *.jar | ForEach-Object { $_.FullName }) -join ';')
$srcFiles = @(
    (Join-Path $BaseDir 'src\com\acugis\reports\ReportServlet.java'),
    (Join-Path $BaseDir 'src\com\acugis\reports\ReportParameterBinder.java'),
    (Join-Path $BaseDir 'src\com\acugis\reports\ReportQueryRunner.java'),
    (Join-Path $BaseDir 'src\com\acugis\reports\ReportQueryServlet.java'),
    (Join-Path $BaseDir 'src\com\acugis\reports\CollectionFill.java'),
    (Join-Path $BaseDir 'src\com\acugis\reports\CollectionQueryExecuterFactory.java'),
    (Join-Path $BaseDir 'src\com\acugis\reports\DuckDBSession.java'),
    (Join-Path $BaseDir 'src\com\acugis\reports\ReportService.java')
)
java -jar $ecj -1.8 -cp $cp -d $classesOut -sourcepath (Join-Path $BaseDir 'src') @($srcFiles)

$dest = Join-Path $work 'WEB-INF\classes\com\acugis\reports'
@(
    'ReportServlet.class',
    'ReportParameterBinder.class',
    'ReportQueryRunner.class',
    'ReportQueryServlet.class',
    'CollectionFill.class',
    'CollectionQueryExecuterFactory.class',
    'DuckDBSession.class',
    'ReportService.class'
) | ForEach-Object {
    $srcClass = Join-Path $classesOut "com\acugis\reports\$_"
    if (-not (Test-Path $srcClass)) {
        # Inner classes (e.g. ReportQueryRunner$SimpleValueParameter.class)
        Get-ChildItem (Join-Path $classesOut 'com\acugis\reports') -Filter ($_ -replace '\.class$','*.class') -ErrorAction SilentlyContinue |
            ForEach-Object { Copy-Item $_.FullName $dest -Force }
    } else {
        Copy-Item $srcClass $dest -Force
    }
}
# Copy any nested classes generated for ReportQueryRunner / ReportQueryServlet
Get-ChildItem (Join-Path $classesOut 'com\acugis\reports') -Filter 'ReportQuery*.class' -ErrorAction SilentlyContinue |
    ForEach-Object { Copy-Item $_.FullName $dest -Force }

# Ensure /reportQuery is mapped in web.xml
$webXml = Join-Path $work 'WEB-INF\web.xml'
$web = Get-Content $webXml -Raw
if ($web -notmatch 'ReportQueryServlet') {
    $snippet = @"

    <servlet>
        <servlet-name>ReportQueryServlet</servlet-name>
        <servlet-class>com.acugis.reports.ReportQueryServlet</servlet-class>
    </servlet>
    <servlet-mapping>
        <servlet-name>ReportQueryServlet</servlet-name>
        <url-pattern>/reportQuery</url-pattern>
    </servlet-mapping>
"@
    $web = $web -replace '(</servlet-mapping>\s*)(\s*<filter>)', "`$1$snippet`$2"
    if ($web -notmatch 'ReportQueryServlet') {
        $web = $web -replace '(</web-app>)', "$snippet`r`n`$1"
    }
    Set-Content -Path $webXml -Value $web -Encoding UTF8
}

# --- PostGIS MapRenderer (acugis-report-map) ---
function Get-JarArtifactKey([string]$fileName) {
    # commons-io-2.11.0.jar -> commons-io ; gt-main-28.2.jar -> gt-main
    if ($fileName -match '^(.*?)-(\d[\w\.\-]*)\.jar$') {
        return $Matches[1].ToLowerInvariant()
    }
    return [IO.Path]::GetFileNameWithoutExtension($fileName).ToLowerInvariant()
}

$existingKeys = @{}
Get-ChildItem $libPath -Filter '*.jar' | ForEach-Object {
    $existingKeys[(Get-JarArtifactKey $_.Name)] = $true
}

$mapJarsAdded = 0
if (Test-Path $mapLib) {
    Get-ChildItem $mapLib -Filter '*.jar' | ForEach-Object {
        $key = Get-JarArtifactKey $_.Name
        $destJar = Join-Path $libPath $_.Name
        # Always refresh the map renderer itself; skip other duplicate artifact ids.
        if ($_.Name -eq 'acugis-report-map.jar') {
            Copy-Item $_.FullName $destJar -Force
            $existingKeys[$key] = $true
            $script:mapJarsAdded++
            Write-Host "Updated map-lib: $($_.Name)"
        } elseif ($existingKeys.ContainsKey($key)) {
            Write-Host "Skip (already in WAR): $($_.Name)"
        } else {
            Copy-Item $_.FullName $destJar -Force
            $existingKeys[$key] = $true
            $script:mapJarsAdded++
            Write-Host "Added map-lib: $($_.Name)"
        }
    }
} else {
    Write-Host "WARN map-lib/ missing - MapRenderer jars were not added to the WAR"
}

$renderer = Join-Path $libPath 'acugis-report-map.jar'
if (Test-Path $renderer) {
    Write-Host "MapRenderer present ($mapJarsAdded new jars from map-lib)"
} else {
    Write-Host "WARN acugis-report-map.jar not in WEB-INF/lib - PostGIS Map elements will fail at fill time"
}

# --- Barcode4J / QR (Designer barcode4j:* components) ---
$barcodeJarsAdded = 0
if (Test-Path $barcodeLib) {
    Get-ChildItem $barcodeLib -Filter '*.jar' | ForEach-Object {
        $key = Get-JarArtifactKey $_.Name
        $destJar = Join-Path $libPath $_.Name
        if ($_.Name -like 'jasperreports-barcode4j-*.jar') {
            Copy-Item $_.FullName $destJar -Force
            $existingKeys[$key] = $true
            $script:barcodeJarsAdded++
            Write-Host "Updated barcode-lib: $($_.Name)"
        } elseif ($existingKeys.ContainsKey($key)) {
            Write-Host "Skip (already in WAR): $($_.Name)"
        } else {
            Copy-Item $_.FullName $destJar -Force
            $existingKeys[$key] = $true
            $script:barcodeJarsAdded++
            Write-Host "Added barcode-lib: $($_.Name)"
        }
    }
} else {
    Write-Host "WARN barcode-lib/ missing - run sync-barcode-lib.ps1 for QR/Barcode support"
}

$barcodeExt = Get-ChildItem $libPath -Filter 'jasperreports-barcode4j-*.jar' | Select-Object -First 1
if ($barcodeExt) {
    Write-Host "Barcode4J present ($barcodeJarsAdded new jars from barcode-lib): $($barcodeExt.Name)"
} else {
    Write-Host "WARN jasperreports-barcode4j not in WEB-INF/lib - barcode4j:QRCode / Code128 will fail at load time"
}

$warNew = Join-Path $BaseDir 'acugis-report-server.war.new'
if (Test-Path $warNew) { Remove-Item $warNew -Force }

$stream = [System.IO.File]::Open($warNew, [System.IO.FileMode]::Create)
$archive = New-Object System.IO.Compression.ZipArchive($stream, [System.IO.Compression.ZipArchiveMode]::Create)
Get-ChildItem $work -Recurse -File | ForEach-Object {
    $rel = $_.FullName.Substring($work.Length + 1).Replace('\', '/')
    $entry = $archive.CreateEntry($rel, [System.IO.Compression.CompressionLevel]::Optimal)
    $entryStream = $entry.Open()
    try {
        $fileStream = [System.IO.File]::OpenRead($_.FullName)
        try { $fileStream.CopyTo($entryStream) } finally { $fileStream.Close() }
    } finally { $entryStream.Close() }
}
$archive.Dispose()
$stream.Close()

$zip = [System.IO.Compression.ZipFile]::OpenRead($warNew)
$backslashCount = ($zip.Entries | Where-Object { $_.FullName -like '*\*' }).Count
$mapEntry = $zip.Entries | Where-Object { $_.FullName -eq 'WEB-INF/lib/acugis-report-map.jar' } | Select-Object -First 1
$barcodeEntry = $zip.Entries | Where-Object { $_.FullName -like 'WEB-INF/lib/jasperreports-barcode4j-*.jar' } | Select-Object -First 1
$zip.Dispose()
if ($backslashCount -gt 0) {
    throw "WAR contains backslash entry names; Tomcat will not deploy it."
}

Move-Item $warNew $war -Force
Remove-Item $work, $classesOut -Recurse -Force
Write-Host "Rebuilt $war"
if ($mapEntry) {
    Write-Host "Verified WEB-INF/lib/acugis-report-map.jar in WAR"
} else {
    Write-Host "WARNING: acugis-report-map.jar was not packaged into the WAR"
}
if ($barcodeEntry) {
    Write-Host "Verified $($barcodeEntry.FullName) in WAR"
} else {
    Write-Host "WARNING: jasperreports-barcode4j was not packaged into the WAR"
}
