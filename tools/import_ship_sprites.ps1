# tools/import_ship_sprites.ps1
# Шаг «принятое -> игра» (спека docs/specs/2026-09-21-угол-корабля-в-метаданных.md §7):
# копирует принятые PNG в web/static/sprites/ (сверка по sha256, а НЕ по имени:
# имена принятых race_<slug>_NN.png и файлов реестра race_<slug>_<word>.png не
# совпадают), печатает строки реестра ShipSprites для вставки ВРУЧНУЮ (только в
# конец) и отчёт. Реестр скрипт НЕ переписывает.
#
# Гвард orient_meta: пару (A, F) берёт ТОЛЬКО при маркере orient_meta:true;
# записи без маркера (принятые старым способом — угол уже запечён в пиксели)
# печатаются как Angle: 0, Flip: false с пометкой «легаси».
#
# Идемпотентен: повторный прогон не копирует уже импортированное (сверка по
# хэшу) и не печатает уже вставленные строки (сверка File по реестру).
#
# Запуск: powershell -File tools/import_ship_sprites.ps1
# Параметры -AcceptedDir/-SpritesDir/-Registry переопределяют пути (нужны
# смоук-тесту import_ship_sprites_smoke.ps1 для прогона на temp-копиях).
param(
    [string]$AcceptedDir = 'ai_drafts/final_accepted/ships',
    [string]$SpritesDir = 'web/static/sprites',
    [string]$Registry = 'internal/models/ship_sprites.go'
)

$ErrorActionPreference = 'Stop'
[Console]::OutputEncoding = [System.Text.Encoding]::UTF8
$inv = [System.Globalization.CultureInfo]::InvariantCulture
$repo = Split-Path -Parent $PSScriptRoot

function Resolve-RepoPath([string]$p) {
    if ([System.IO.Path]::IsPathRooted($p)) { return $p }
    return (Join-Path $repo $p)
}
$AcceptedDir = Resolve-RepoPath $AcceptedDir
$SpritesDir = Resolve-RepoPath $SpritesDir
$Registry = Resolve-RepoPath $Registry

function Get-Sha256([string]$path) {
    return (Get-FileHash -LiteralPath $path -Algorithm SHA256).Hash.ToLowerInvariant()
}

# Normalize-ShipAngle — привести угол к конвенции показа (−180, 180], шаг 0.1
# (как shipAngleNorm в студии, спека §3.1): импорт не должен быть второй точкой
# входа с произвольным углом.
function Normalize-ShipAngle([double]$a) {
    while ($a -gt 180) { $a -= 360 }
    while ($a -le -180) { $a += 360 }
    $v = [math]::Round($a * 10) / 10
    if ($v -eq 0) { return 0.0 }
    return $v
}

$metaPath = Join-Path $AcceptedDir 'ships_meta.json'
if (-not (Test-Path -LiteralPath $metaPath)) {
    Write-Output "нет $metaPath"
    exit 1
}
$raw = Get-Content -LiteralPath $metaPath -Raw -Encoding UTF8
$parsed = ConvertFrom-Json -InputObject $raw
$records = @($parsed)

# Карта sha256 -> имя файла в игре: сопоставление по СОДЕРЖИМОМУ, не по имени.
$hashToName = @{}
foreach ($f in Get-ChildItem -LiteralPath $SpritesDir -Filter *.png -File) {
    $hashToName[(Get-Sha256 $f.FullName)] = $f.Name
}

$registryText = ''
if (Test-Path -LiteralPath $Registry) { $registryText = Get-Content -LiteralPath $Registry -Raw -Encoding UTF8 }
function Test-InRegistry([string]$name) {
    return $registryText -match [regex]::Escape('File: "' + $name + '"')
}

$already = 0; $copied = 0; $legacy = 0; $rows = 0; $errors = 0; $idx = 0
foreach ($rec in $records) {
    $idx++
    $file = [string]$rec.file
    $src = Join-Path $AcceptedDir $file
    if (-not (Test-Path -LiteralPath $src)) { Write-Output ("пропуск: нет файла " + $file); continue }
    $hash = Get-Sha256 $src
    $gameName = $null

    if ($hashToName.ContainsKey($hash)) {
        $gameName = $hashToName[$hash]
        $already++
        Write-Output ("уже в игре как " + $gameName)
    } else {
        $dst = Join-Path $SpritesDir $file
        if (Test-Path -LiteralPath $dst) {
            if ((Get-Sha256 $dst) -eq $hash) {
                $gameName = $file; $already++
                Write-Output ("уже в игре как " + $gameName)
            } else {
                $errors++
                Write-Output ("ОШИБКА: " + $file + " - имя занято файлом с другим хэшем (не перезаписываю)")
            }
        } else {
            Copy-Item -LiteralPath $src -Destination $dst
            $hashToName[$hash] = $file
            $gameName = $file
            $copied++
            Write-Output ("скопировано: " + $gameName)
        }
    }
    if ($null -eq $gameName) { continue }

    # Пара (A, F) — только при маркере orient_meta; иначе пиксели уже довёрнуты.
    # Угол нормализуется в (−180, 180] (как shipAngleNorm в студии).
    if ($rec.orient_meta -eq $true) {
        $a = Normalize-ShipAngle ([double]$rec.angle)
        $f = [bool]$rec.flip
    } else {
        $a = 0.0; $f = $false; $legacy++
        Write-Output ("легаси: пара обнулена (ориентация уже в пикселях): " + $file)
    }
    $aStr = $a.ToString($inv)
    $fStr = if ($f) { 'true' } else { 'false' }

    # Имя корабля — автомат «<имя расы> · <NN>» (NN из имени принятого файла).
    $nn = ''
    $m = [regex]::Match($file, '_(\d+)\.png$')
    if ($m.Success) { $nn = $m.Groups[1].Value } else { $nn = ('{0:D2}' -f $idx) }
    $id = $gameName -replace '\.png$', ''
    $name = ([string]$rec.race_name) + ' · ' + $nn

    if (Test-InRegistry $gameName) { continue }  # строка уже в реестре — не печатаем
    Write-Output ('{ID: "' + $id + '", Name: "' + $name + '", File: "' + $gameName + '", Angle: ' + $aStr + ', Flip: ' + $fStr + '},')
    $rows++
}

Write-Output '---'
Write-Output ("уже в игре: " + $already)
Write-Output ("скопировано: " + $copied)
Write-Output ("легаси: пара обнулена: " + $legacy)
Write-Output ("нужна строка в реестр: " + $rows)
if ($errors -gt 0) { Write-Output ("ошибок: " + $errors) }
