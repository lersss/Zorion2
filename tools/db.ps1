# One-shot psql wrapper for Zorion (Windows PowerShell 5.1).
#
# Why: agents repeatedly re-invent the psql call and get stuck in loops -
# cp866/cp1251 output encoding, split arguments, password, multi-line SQL.
# The loop-guard log recorded dozens of repeats of the same command.
# This script does it once, correctly, and gives a single short call.
#
# Usage:
#   powershell -File tools/db.ps1 -Sql "SELECT 1"
#   powershell -File tools/db.ps1 -File my_query.sql
#   echo "SELECT count(*) FROM planets" | powershell -File tools/db.ps1
#
# Reads from the dev DB by default (DATABASE_URL is not parsed; creds below).

param(
    [string]$Sql,
    [string]$File,
    [string]$DbName = "zorion",
    [string]$DbUser = "zorion",
    [string]$DbPassword = "zorion123",
    [string]$DbHost = "127.0.0.1",
    [int]$DbPort = 5432
)

$ErrorActionPreference = "Stop"
[Console]::OutputEncoding = [System.Text.Encoding]::UTF8
$OutputEncoding = [System.Text.Encoding]::UTF8

$psql = "C:\pgsql\pgsql\bin\psql.exe"
if (-not (Test-Path -LiteralPath $psql)) {
    Write-Error "psql not found: $psql"
    exit 1
}

# SQL source: -Sql, -File, or stdin.
if ($Sql) {
    $query = $Sql
} elseif ($File) {
    if (-not (Test-Path -LiteralPath $File)) {
        Write-Error "SQL file not found: $File"
        exit 1
    }
    $query = Get-Content -LiteralPath $File -Raw -Encoding UTF8
} else {
    $query = [Console]::In.ReadToEnd()
}

if (-not $query -or -not $query.Trim()) {
    Write-Error "No SQL given: use -Sql, -File or stdin."
    exit 1
}

# Write SQL to a temp UTF-8 .sql file and call psql via cmd:
# this avoids PowerShell breaking multi-line queries and arguments.
$tmp = [System.IO.Path]::GetTempFileName() + ".sql"
$code = 0
try {
    Set-Content -LiteralPath $tmp -Value $query -Encoding UTF8
    $env:PGPASSWORD = $DbPassword
    $env:PGCLIENTENCODING = "UTF8"
    & cmd /c "`"$psql`" -h $DbHost -p $DbPort -U $DbUser -d $DbName -v ON_ERROR_STOP=1 -f `"$tmp`" 2>&1"
    $code = $LASTEXITCODE
} finally {
    Remove-Item -LiteralPath $tmp -ErrorAction SilentlyContinue
    Remove-Item Env:\PGPASSWORD -ErrorAction SilentlyContinue
}

exit $code
