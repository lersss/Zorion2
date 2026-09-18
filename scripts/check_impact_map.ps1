# check_impact_map.ps1 — автосверка реестра каскадных влияний docs/impact_map.json
# Критерии валидности — идея 83a §3; механика сверки с git — §6.1.
# Только читает и проверяет, файлы не модифицирует.

param(
    [string]$RepoRoot = ""
)

$ErrorActionPreference = 'Stop'

# UTF-8 вывод (PS 5.1 по умолчанию пишет в cp866/cp1251)
[Console]::OutputEncoding = [System.Text.Encoding]::UTF8

# --- корень репо: параметр или родитель папки скрипта ---
if (-not $RepoRoot) {
    $RepoRoot = Split-Path -Parent $PSScriptRoot
}
$RepoRoot = (Resolve-Path $RepoRoot).Path

$mapPath = Join-Path $RepoRoot 'docs/impact_map.json'

$errors = @()
$warnings = @()

# --- 1. JSON парсится ---
if (-not (Test-Path -LiteralPath $mapPath)) {
    Write-Host "[impact-map] ERROR: не найден $mapPath" -ForegroundColor Red
    Write-Host "ERROR: 1, WARNING: 0"
    exit 1
}
try {
    $data = Get-Content -LiteralPath $mapPath -Raw -Encoding UTF8 | ConvertFrom-Json
} catch {
    Write-Host "[impact-map] ERROR: JSON не парсится: $($_.Exception.Message)" -ForegroundColor Red
    Write-Host "ERROR: 1, WARNING: 0"
    exit 1
}

# --- 2. schema_version и updated_at ---
if ($data.schema_version -isnot [int] -or $data.schema_version -lt 1) {
    $errors += "schema_version должен быть целым >= 1 (найдено: $($data.schema_version))"
}
if ($data.updated_at -notmatch '^\d{4}-\d{2}-\d{2}$') {
    $errors += "updated_at должен быть YYYY-MM-DD (найдено: $($data.updated_at))"
}

$validTypes = @('config','table','balance','spec','doc','test','art','api','code','migration','mechanic')
$validKinds = @('reads','writes','duplicates','derives','schema','test','api','docs','affects')

$entities = @($data.entities)

# --- 3. entity.id уникальны ---
foreach ($g in ($entities | Group-Object -Property id)) {
    if ($g.Count -gt 1) {
        $errors += "дубль entity.id: $($g.Name)"
    }
}

$allIds = @{}
foreach ($e in $entities) { $allIds[$e.id] = $true }

# --- проверка существования пути/glob от корня репо ---
function Test-PathExists {
    param([string]$Root, [string]$RelPath)
    $full = Join-Path $Root $RelPath
    if ($RelPath -match '[*?]') {
        return ((@(Get-ChildItem -Path $full -ErrorAction SilentlyContinue)).Count -ge 1)
    }
    return (Test-Path -LiteralPath $full)
}

# --- 4-7. по каждой сущности ---
foreach ($e in $entities) {
    if ($validTypes -notcontains $e.type) {
        $errors += "[$($e.id)] type вне списка: $($e.type)"
    }
    if ($e.path -notlike 'db:*') {
        if (-not (Test-PathExists $RepoRoot $e.path)) {
            $errors += "реестр протух: $($e.path)"
        }
    }
    $seenPairs = @{}
    foreach ($imp in @($e.impacts)) {
        if ($validKinds -notcontains $imp.kind) {
            $errors += "[$($e.id)] impact.kind вне списка: $($imp.kind)"
        }
        $on = $imp.on
        if (-not $allIds.ContainsKey($on) -and $on -notlike 'db:*') {
            if (-not (Test-PathExists $RepoRoot $on)) {
                $errors += "реестр протух: $on"
            }
        }
        $pairKey = "$on|$($imp.kind)"
        if ($seenPairs.ContainsKey($pairKey)) {
            $errors += "[$($e.id)] дубль пары on+kind: $on / $($imp.kind)"
        } else {
            $seenPairs[$pairKey] = $true
        }
    }
}

# --- сверка с git: упомянутые пути (path сущностей + on-пути, не-db, не-id) ---
$mentioned = @()
foreach ($e in $entities) {
    if ($e.path -notlike 'db:*') { $mentioned += $e.path }
    foreach ($imp in @($e.impacts)) {
        $on = $imp.on
        if (-not $allIds.ContainsKey($on) -and $on -notlike 'db:*') {
            $mentioned += $on
        }
    }
}

$exceptions = @(
    '.opencode/', 'docs/gamedesign/ideas/', 'scripts/', '.githooks/', 'docs/impact_map.json',
    # код реализации и вспомогательные зоны — не сущности реестра (2026-09-18, 77a/88a/90a)
    'internal/', 'web/', 'cmd/', 'tools/', 'migrations/',
    'docs/QA/checklists/', 'docs/QA_CHECKLIST.md',
    'docs/ARCHITECTURE.md', 'docs/PITFALLS.md',
    'AGENTS.md', 'docs/INDEX.md',
    'go.mod', 'go.sum', '.gitignore'
)

$changedFiles = @()
try {
    $changedFiles = @(& git -C $RepoRoot -c core.quotepath=false log --name-only -n 20 --pretty=format: 2>$null | Where-Object { $_ -and $_.Trim() })
} catch {
    # git недоступен или не репозиторий — сверку с git пропускаем
}
foreach ($f in $changedFiles) {
    $f = $f.Trim()
    if (-not $f) { continue }
    $isException = $false
    foreach ($ex in $exceptions) {
        if ($f.StartsWith($ex)) { $isException = $true; break }
    }
    if ($isException) { continue }
    $isMentioned = $false
    foreach ($m in $mentioned) {
        if ($f -eq $m -or $f -like $m) { $isMentioned = $true; break }
    }
    if (-not $isMentioned) {
        $warnings += "возможно, новая горячая точка: $f"
    }
}

# --- вывод ---
Write-Host ""
Write-Host "=== impact-map: сверка реестра ==="
if ($errors.Count -gt 0) {
    Write-Host "--- ERROR ---" -ForegroundColor Red
    foreach ($e in $errors) { Write-Host "  ERROR: $e" -ForegroundColor Red }
}
if ($warnings.Count -gt 0) {
    Write-Host "--- WARNING ---" -ForegroundColor Yellow
    foreach ($w in $warnings) { Write-Host "  WARNING: $w" -ForegroundColor Yellow }
}
Write-Host "ERROR: $($errors.Count), WARNING: $($warnings.Count)"
if ($errors.Count -gt 0) { exit 1 } else { exit 0 }
