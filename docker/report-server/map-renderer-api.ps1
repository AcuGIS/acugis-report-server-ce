# Shared MapRenderer contract check for the Report Server WAR build.
# The current report generator calls the 9-argument overload:
#   render(Connection, String, int, int, String, String, String, String latitudeField, String longitudeField)

function Assert-MapRendererNineArgApi {
    param(
        [Parameter(Mandatory = $true)]
        [string]$JarPath
    )
    if (-not (Test-Path -LiteralPath $JarPath)) {
        throw "MapRenderer jar not found: $JarPath"
    }
    $javapCmd = Get-Command javap -ErrorAction SilentlyContinue
    if (-not $javapCmd) {
        throw "javap is required to verify MapRenderer.render. Install a JDK and ensure javap is on PATH."
    }
    $output = & javap -public -classpath $JarPath com.acugis.map.MapRenderer 2>&1 | Out-String
    if ($LASTEXITCODE -ne 0) {
        throw "javap failed for ${JarPath}:`n$output"
    }
    $flat = ($output -replace '\s+', '')
    $needle = 'render(java.sql.Connection,java.lang.String,int,int,java.lang.String,java.lang.String,java.lang.String,java.lang.String,java.lang.String)'
    if ($flat -notlike "*$needle*") {
        throw @"
acugis-report-map.jar does not expose the 9-argument MapRenderer.render overload required by the current report generator:
  render(Connection, String, int, int, String, String, String, String latitudeField, String longitudeField)
The jar must expose the 9-argument MapRenderer.render overload.
Jar: $JarPath
javap:
$output
"@
    }
}

function Assert-WarMapRendererNineArgApi {
    param(
        [Parameter(Mandatory = $true)]
        [string]$WarPath
    )
    if (-not (Test-Path -LiteralPath $WarPath)) {
        throw "WAR not found: $WarPath"
    }
    Add-Type -AssemblyName System.IO.Compression.FileSystem
    $zip = [System.IO.Compression.ZipFile]::OpenRead((Resolve-Path -LiteralPath $WarPath).Path)
    $tmp = Join-Path ([System.IO.Path]::GetTempPath()) ("acugis-report-map-verify-" + [guid]::NewGuid().ToString('n') + '.jar')
    try {
        $entry = $zip.GetEntry('WEB-INF/lib/acugis-report-map.jar')
        if (-not $entry) {
            throw "WAR is missing WEB-INF/lib/acugis-report-map.jar: $WarPath"
        }
        $inStream = $entry.Open()
        try {
            $outStream = [System.IO.File]::Create($tmp)
            try { $inStream.CopyTo($outStream) } finally { $outStream.Dispose() }
        } finally { $inStream.Dispose() }
    } finally {
        $zip.Dispose()
    }
    try {
        Assert-MapRendererNineArgApi -JarPath $tmp
    } finally {
        Remove-Item -LiteralPath $tmp -Force -ErrorAction SilentlyContinue
    }
}
