# Build standalone Tomcat 10 WAR → dist/acugis-report-server.war
# Profile: standalone (does NOT modify docker/report-server)
#
# Usage (from stand-alone-tomcat-10):
#   powershell -File .\build.ps1
#
# Optional: prefers Maven if available (`mvn -Pstandalone package`), otherwise
# rebuilds from the Docker WAR libs + Jakarta servlet module + local sources.

param(
    [string]$BaseDir = $PSScriptRoot,
    [switch]$SkipMaven
)

$ErrorActionPreference = 'Stop'
$RepoRoot = Split-Path $BaseDir -Parent
$DockerDir = Join-Path $RepoRoot 'docker\report-server'
$DistDir = Join-Path $BaseDir 'dist'
$Work = Join-Path $BaseDir '_build_work'
$ClassesOut = Join-Path $BaseDir '_classes_out'
$Tools = Join-Path $BaseDir 'lib'
$Ecj = Join-Path $Tools '_ecj.jar'
$ServletApi = Join-Path $Tools 'jakarta.servlet-api-5.0.0.jar'
$JakartaServlets = Join-Path $Tools 'jasperreports-servlets-7.0.1.jar'
$JasperJdt = Join-Path $Tools 'jasperreports-jdt-7.0.1.jar'
$MapLib = Join-Path $DockerDir 'map-lib'
$BarcodeLib = Join-Path $DockerDir 'barcode-lib'
$WarBak = Join-Path $DockerDir 'acugis-report-server.war.bak'
$WarLive = Join-Path $DockerDir 'acugis-report-server.war'
$SrcRoot = Join-Path $BaseDir 'src\main\java'
$WebApp = Join-Path $BaseDir 'src\main\webapp'
$Resources = Join-Path $BaseDir 'src\main\resources'

function Ensure-Dir([string]$Path) {
    if (-not (Test-Path $Path)) {
        New-Item -ItemType Directory -Path $Path | Out-Null
    }
}

function Download-File([string]$Uri, [string]$OutFile) {
    if (Test-Path $OutFile) { return }
    Ensure-Dir (Split-Path $OutFile -Parent)
    Write-Host "Downloading $Uri"
    Invoke-WebRequest -Uri $Uri -OutFile $OutFile -UseBasicParsing
}

function Get-JarArtifactKey([string]$fileName) {
    if ($fileName -match '^(.*?)-(\d[\w\.\-]*)\.jar$') {
        return $Matches[1].ToLowerInvariant()
    }
    return [IO.Path]::GetFileNameWithoutExtension($fileName).ToLowerInvariant()
}

function Build-WithMaven {
    $mvn = Get-Command mvn -ErrorAction SilentlyContinue
    if (-not $mvn) { return $false }
    Write-Host "Maven detected - running: mvn -Pstandalone -DskipTests package"
    Push-Location $BaseDir
    try {
        & mvn -Pstandalone -DskipTests package
        if ($LASTEXITCODE -ne 0) { throw "Maven build failed with exit code $LASTEXITCODE" }
    } finally {
        Pop-Location
    }
    return $true
}

function Build-WithEcj {
    $baseWar = if (Test-Path $WarBak) { $WarBak } elseif (Test-Path $WarLive) { $WarLive } else { $null }
    if (-not $baseWar) {
        throw "Missing base WAR at $WarBak or $WarLive (needed for Jasper dependency jars)"
    }

    Ensure-Dir $Tools
    Download-File 'https://repo1.maven.org/maven2/org/eclipse/jdt/ecj/3.33.0/ecj-3.33.0.jar' $Ecj
    Download-File 'https://repo1.maven.org/maven2/jakarta/servlet/jakarta.servlet-api/5.0.0/jakarta.servlet-api-5.0.0.jar' $ServletApi
    Download-File 'https://repo1.maven.org/maven2/net/sf/jasperreports/jasperreports-servlets/7.0.1/jasperreports-servlets-7.0.1.jar' $JakartaServlets
    Download-File 'https://repo1.maven.org/maven2/net/sf/jasperreports/jasperreports-jdt/7.0.1/jasperreports-jdt-7.0.1.jar' $JasperJdt
    # ECJ runtime used by jasperreports-jdt (keep separate from build-only _ecj.jar)
    $EcjRuntime = Join-Path $Tools 'ecj-3.33.0.jar'
    Download-File 'https://repo1.maven.org/maven2/org/eclipse/jdt/ecj/3.33.0/ecj-3.33.0.jar' $EcjRuntime

    foreach ($p in @($Work, $ClassesOut)) {
        if (Test-Path $p) { Remove-Item $p -Recurse -Force }
        New-Item -ItemType Directory -Path $p | Out-Null
    }

    Add-Type -AssemblyName System.IO.Compression.FileSystem
    [System.IO.Compression.ZipFile]::ExtractToDirectory($baseWar, $Work)

    $libPath = Join-Path $Work 'WEB-INF\lib'
    $classesPath = Join-Path $Work 'WEB-INF\classes'

    # Remove Java EE servlet module; add Jakarta module
    Get-ChildItem $libPath -Filter 'jasperreports-servlets-j2ee*.jar' -ErrorAction SilentlyContinue |
        ForEach-Object { Remove-Item $_.FullName -Force; Write-Host "Removed $($_.Name)" }
    Copy-Item $JakartaServlets (Join-Path $libPath 'jasperreports-servlets-7.0.1.jar') -Force
    Write-Host "Installed jasperreports-servlets-7.0.1.jar (Jakarta)"
    Copy-Item $JasperJdt (Join-Path $libPath 'jasperreports-jdt-7.0.1.jar') -Force
    Copy-Item $EcjRuntime (Join-Path $libPath 'ecj-3.33.0.jar') -Force
    Write-Host "Installed jasperreports-jdt-7.0.1.jar + ecj-3.33.0.jar (JRXML compile)"

    # Refresh map-lib jars
    $existingKeys = @{}
    Get-ChildItem $libPath -Filter '*.jar' | ForEach-Object {
        $existingKeys[(Get-JarArtifactKey $_.Name)] = $true
    }
    if (Test-Path $MapLib) {
        Get-ChildItem $MapLib -Filter '*.jar' | ForEach-Object {
            $key = Get-JarArtifactKey $_.Name
            $destJar = Join-Path $libPath $_.Name
            if ($_.Name -eq 'acugis-report-map.jar' -or -not $existingKeys.ContainsKey($key)) {
                Copy-Item $_.FullName $destJar -Force
                $existingKeys[$key] = $true
                Write-Host "map-lib: $($_.Name)"
            }
        }
    } else {
        Write-Host "WARN: map-lib missing at $MapLib"
    }

    # Barcode4J / QR (Designer barcode4j:* Jackson component kinds)
    if (Test-Path $BarcodeLib) {
        Get-ChildItem $BarcodeLib -Filter '*.jar' | ForEach-Object {
            $key = Get-JarArtifactKey $_.Name
            $destJar = Join-Path $libPath $_.Name
            if ($_.Name -like 'jasperreports-barcode4j-*.jar' -or -not $existingKeys.ContainsKey($key)) {
                Copy-Item $_.FullName $destJar -Force
                $existingKeys[$key] = $true
                Write-Host "barcode-lib: $($_.Name)"
            }
        }
    } else {
        Write-Host "WARN: barcode-lib missing at $BarcodeLib (run docker/report-server/sync-barcode-lib.ps1)"
    }

    # JDBC drivers are NOT packaged into the WAR — install into CATALINA_HOME/lib (or CATALINA_BASE/lib).
    # Strip any JDBC jars that may have come from the Docker base WAR.
    $jdbcNamePattern = 'postgresql-|mysql-connector|mysql-jdbc|mariadb-java-client|mssql-jdbc|sqljdbc|ojdbc|sqlite-jdbc|snowflake-jdbc|db2jcc|jcc-|duckdb_jdbc-'
    Get-ChildItem $libPath -Filter '*.jar' -ErrorAction SilentlyContinue | ForEach-Object {
        if ($_.Name -match $jdbcNamePattern) {
            Remove-Item $_.FullName -Force
            Write-Host "Removed JDBC from WAR: $($_.Name)"
        }
    }

    # Compile all standalone sources
    $cpParts = @($ServletApi)
    $cpParts += (Get-ChildItem $libPath -Filter '*.jar' | ForEach-Object { $_.FullName })
    $cp = ($cpParts -join ';')
    $srcFiles = Get-ChildItem $SrcRoot -Recurse -Filter '*.java' | ForEach-Object { $_.FullName }
    if (-not $srcFiles -or $srcFiles.Count -eq 0) {
        throw "No Java sources under $SrcRoot"
    }

    Write-Host "Compiling $($srcFiles.Count) source files..."
    & java -jar $Ecj -11 -cp $cp -d $ClassesOut -sourcepath $SrcRoot @($srcFiles)
    if ($LASTEXITCODE -ne 0) { throw "ECJ compilation failed" }

    # Replace application classes
    $destPkg = Join-Path $classesPath 'com\acugis\reports'
    Ensure-Dir $destPkg
    Get-ChildItem (Join-Path $ClassesOut 'com\acugis\reports') -Filter '*.class' -Recurse | ForEach-Object {
        $rel = $_.FullName.Substring((Join-Path $ClassesOut 'com\acugis\reports').Length).TrimStart('\')
        $target = Join-Path $destPkg $rel
        Ensure-Dir (Split-Path $target -Parent)
        Copy-Item $_.FullName $target -Force
    }

    # web.xml / context.xml / resources from standalone tree
    Copy-Item (Join-Path $WebApp 'WEB-INF\web.xml') (Join-Path $Work 'WEB-INF\web.xml') -Force
    Ensure-Dir (Join-Path $Work 'META-INF')
    Copy-Item (Join-Path $WebApp 'META-INF\context.xml') (Join-Path $Work 'META-INF\context.xml') -Force
    Ensure-Dir (Join-Path $Work 'WEB-INF\license')
    if (Test-Path $Resources) {
        # Copy classpath resources recursively (fonts/, *.properties, …)
        Get-ChildItem $Resources -Recurse -File | ForEach-Object {
            $rel = $_.FullName.Substring($Resources.Length).TrimStart('\', '/')
            $target = Join-Path $classesPath $rel
            Ensure-Dir (Split-Path $target -Parent)
            Copy-Item $_.FullName $target -Force
        }
    }

    # Remove any leftover javax-only debug noise is fine; package WAR with forward slashes
    Ensure-Dir $DistDir
    $warOut = Join-Path $DistDir 'acugis-report-server.war'
    $warTmp = Join-Path $DistDir 'acugis-report-server.war.tmp'
    if (Test-Path $warTmp) { Remove-Item $warTmp -Force }
    if (Test-Path $warOut) { Remove-Item $warOut -Force }

    Add-Type -AssemblyName System.IO.Compression
    $stream = [System.IO.File]::Open($warTmp, [System.IO.FileMode]::Create)
    $archive = New-Object System.IO.Compression.ZipArchive($stream, [System.IO.Compression.ZipArchiveMode]::Create)
    Get-ChildItem $Work -Recurse -File | ForEach-Object {
        $rel = $_.FullName.Substring($Work.Length + 1).Replace('\', '/')
        $entry = $archive.CreateEntry($rel, [System.IO.Compression.CompressionLevel]::Optimal)
        $entryStream = $entry.Open()
        try {
            $fileStream = [System.IO.File]::OpenRead($_.FullName)
            try { $fileStream.CopyTo($entryStream) } finally { $fileStream.Close() }
        } finally { $entryStream.Close() }
    }
    $archive.Dispose()
    $stream.Close()

    $zip = [System.IO.Compression.ZipFile]::OpenRead($warTmp)
    $backslashCount = ($zip.Entries | Where-Object { $_.FullName -like '*\*' }).Count
    $hasJakarta = ($zip.Entries | Where-Object { $_.FullName -eq 'WEB-INF/lib/jasperreports-servlets-7.0.1.jar' }).Count -gt 0
    $hasJ2ee = ($zip.Entries | Where-Object { $_.FullName -like 'WEB-INF/lib/jasperreports-servlets-j2ee*' }).Count -gt 0
    $jdbcPacked = @($zip.Entries | Where-Object {
        $_.FullName -like 'WEB-INF/lib/*' -and (
            $_.FullName -match 'postgresql-|mysql-connector|mysql-jdbc|mariadb-java-client|mssql-jdbc|sqljdbc|ojdbc|sqlite-jdbc|snowflake-jdbc|db2jcc|jcc-|duckdb_jdbc-'
        )
    }).Count
    $zip.Dispose()
    if ($backslashCount -gt 0) { throw "WAR contains backslash entry names" }
    if (-not $hasJakarta) { throw "Jakarta jasperreports-servlets missing from WAR" }
    if ($hasJ2ee) { throw "j2ee servlet module must not be present in standalone WAR" }
    if ($jdbcPacked -gt 0) {
        throw "JDBC driver jars must not be packaged in WEB-INF/lib (place them in CATALINA_HOME/lib)"
    }
    Write-Host "Verified WAR has no JDBC driver jars (install into CATALINA_HOME/lib)"

    Move-Item $warTmp $warOut -Force
    Remove-Item $Work, $ClassesOut -Recurse -Force
    Write-Host "Built $warOut"
}

# --- main ---
. (Join-Path $DockerDir 'map-renderer-api.ps1')
Assert-MapRendererNineArgApi -JarPath (Join-Path $MapLib 'acugis-report-map.jar')

Ensure-Dir $DistDir
$usedMaven = $false
if (-not $SkipMaven) {
    try { $usedMaven = Build-WithMaven } catch {
        Write-Host "Maven build unavailable/failed; falling back to ECJ packaging. $_"
        $usedMaven = $false
    }
}
if (-not $usedMaven) {
    Build-WithEcj
}

$warOut = Join-Path $DistDir 'acugis-report-server.war'
Assert-WarMapRendererNineArgApi -WarPath $warOut
Write-Host "Verified WEB-INF/lib/acugis-report-map.jar exposes the 9-argument MapRenderer.render overload"

Copy-Item (Join-Path $BaseDir 'sample-acugis.properties') (Join-Path $DistDir 'sample-acugis.properties') -Force
Copy-Item (Join-Path $BaseDir 'dist-README.txt') (Join-Path $DistDir 'README.txt') -Force

Write-Host ""
Write-Host "Standalone dist output:"
Get-ChildItem $DistDir | ForEach-Object {
    $sizeMb = [math]::Round(($_.Length / 1024.0 / 1024.0), 2)
    Write-Host ("  " + $_.Name + "  (" + $sizeMb + " MB)")
}
Write-Host ""
Write-Host ("Docker tree untouched: " + $DockerDir)
