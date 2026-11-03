# Download JasperReports Barcode4J / QR jars into docker/report-server/barcode-lib/
# for packaging into acugis-report-server.war (WEB-INF/lib).
#
# Designer Preview already has these (tools/jasper-preview/sync-libs.ps1).
# Without them, Jackson fails with:
#   Could not resolve type id 'barcode4j:QRCode' as a subtype of Component
#   known type ids = [iconLabel, spiderChart]
#
# Usage (from docker/report-server):
#   powershell -File .\sync-barcode-lib.ps1
#   powershell -File .\rebuild-war.ps1

param(
    [string]$BaseDir = $PSScriptRoot
)

$ErrorActionPreference = 'Stop'
$barcodeLib = Join-Path $BaseDir 'barcode-lib'
New-Item -ItemType Directory -Force -Path $barcodeLib | Out-Null

$artifacts = @(
    @{ g = 'net.sf.jasperreports'; a = 'jasperreports-barcode4j'; v = '7.0.1' },
    @{ g = 'net.sf.barcode4j'; a = 'barcode4j'; v = '2.1' },
    @{ g = 'com.google.zxing'; a = 'core'; v = '3.4.0' }
)

foreach ($x in $artifacts) {
    $name = "$($x.a)-$($x.v).jar"
    $out = Join-Path $barcodeLib $name
    if (Test-Path $out) {
        Write-Host "OK  $name"
        continue
    }
    $path = "$($x.g.Replace('.','/'))/$($x.a)/$($x.v)/$name"
    $url = "https://repo1.maven.org/maven2/$path"
    Write-Host "GET $url"
    Invoke-WebRequest -Uri $url -OutFile $out -UseBasicParsing
}

$ext = Get-ChildItem $barcodeLib -Filter 'jasperreports-barcode4j-*.jar' | Select-Object -First 1
$lib = Get-ChildItem $barcodeLib -Filter 'barcode4j-*.jar' | Select-Object -First 1
$zxing = Get-ChildItem $barcodeLib -Filter 'core-3*.jar' | Select-Object -First 1
if (-not $ext) { throw 'jasperreports-barcode4j jar missing after sync' }
if (-not $lib) { throw 'barcode4j jar missing after sync' }
if (-not $zxing) { throw 'zxing core jar missing after sync (required for QRCode)' }

Write-Host "barcode-lib jar count:" (Get-ChildItem $barcodeLib -Filter '*.jar').Count
Write-Host "Barcode4J extension:" $ext.Name
Write-Host "Barcode4J library:" $lib.Name
Write-Host "ZXing core:" $zxing.Name
Write-Host "Next: powershell -File .\rebuild-war.ps1"
