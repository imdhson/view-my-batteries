$ErrorActionPreference = "Stop"

$AppName = "view-my-batteries"
$LogFile = "app.log"
$ExeName = "$AppName.exe"

# Determine port from ADDR environment variable or default to 8080
$Addr = [Environment]::GetEnvironmentVariable("ADDR")
if (-not $Addr) {
    $Port = 8080
} else {
    $PortMatch = [regex]::Match($Addr, ':(\d+)$')
    if ($PortMatch.Success) {
        $Port = [int]$PortMatch.Groups[1].Value
    } else {
        $Port = 8080
    }
}

Write-Host "Building $AppName..."
go build -o $ExeName .

Write-Host "Stopping any existing instance of $AppName..."
# Stop by process name (without .exe extension)
Get-Process -Name $AppName -ErrorAction SilentlyContinue | Stop-Process -Force -ErrorAction SilentlyContinue

# Try to find and kill process by port using netstat
$NetstatOutput = netstat -ano | Select-String ":$Port\s"
if ($NetstatOutput) {
    $NetstatOutput | ForEach-Object {
        $Line = $_.ToString().Trim() -replace '\s+', ' '
        $Parts = $Line.Split(' ')
        $PidValue = $Parts[-1]

        if ([int]::TryParse($PidValue, [ref]$null) -and $PidValue -ne "0") {
            Write-Host "Killing process $PidValue on port $Port..."
            Stop-Process -Id $PidValue -Force -ErrorAction SilentlyContinue
        }
    }
}

Write-Host "Starting $AppName..."
# Start process in background and redirect output
Start-Process -FilePath ".\$ExeName" -NoNewWindow -RedirectStandardOutput $LogFile -RedirectStandardError $LogFile -PassThru | Out-Null

Write-Host "Deployed successfully! Logs are being written to $LogFile."
