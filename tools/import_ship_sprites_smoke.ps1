# tools/import_ship_sprites_smoke.ps1
# Смоук импорта принятых кораблей (спеки docs/specs/2026-09-21-угол-корабля-в-метаданных.md
# §9 п.18, docs/specs/2026-09-23-корабли-рас-раса-агентов-и-игрока.md §6.1 п.1/2/6 и
# docs/specs/2026-09-25-арт-студия-удаление-кораблей-из-игры.md §3.4).
# Работает на temp-копиях: реальные ai_drafts/final_accepted и web/static/sprites
# НЕ трогаются. Реестр — JSON-фикстура (config/ships_registry.json). Проверяет:
# копирование по хэшу, гвард orient_meta (легаси -> Angle 0, Flip false),
# нормализацию угла, конфликт имён (без перезаписи), фильтр (сироты и
# race_humans_01..06), дописывание записей в реестр, канонический порядок,
# сохранение неизвестных полей (scale_human), отсутствие BOM, идемпотентность.
# Вывод ASCII (cp866). Exit 0 — PASS, 1 — FAIL.
# Запуск: powershell -File tools/import_ship_sprites_smoke.ps1
$ErrorActionPreference = 'Stop'
[Console]::OutputEncoding = [System.Text.Encoding]::UTF8

$repo = Split-Path -Parent $PSScriptRoot
$import = Join-Path $PSScriptRoot 'import_ship_sprites.ps1'
$spritesSrc = Join-Path $repo 'web/static/sprites'

$tmp = Join-Path $env:TEMP ('ship_import_smoke_' + [Guid]::NewGuid().ToString('N'))
$accDir = Join-Path $tmp 'accepted'
$sprDir = Join-Path $tmp 'sprites'
$regFile = Join-Path $tmp 'ships_registry.json'
New-Item -ItemType Directory -Path $accDir, $sprDir -Force | Out-Null

# 7 PNG с РАЗНЫМ содержимым: 5 принятых + конфликт-цель + легаси-люди.
$seen = @{}
$pool = @()
foreach ($f in Get-ChildItem -LiteralPath $spritesSrc -Filter *.png -File) {
    $h = (Get-FileHash -LiteralPath $f.FullName).Hash
    if ($seen.ContainsKey($h)) { continue }
    $seen[$h] = $true
    $pool += $f
    if ($pool.Count -ge 7) { break }
}
if ($pool.Count -lt 7) { Write-Output 'RESULT: FAIL - need >=7 distinct source sprites'; exit 1 }

# Принятые: порядок меты намеренно перемешан (проверяем порядок реестра).
Copy-Item -LiteralPath $pool[0].FullName -Destination (Join-Path $accDir 'race_humans_starship.png')
Copy-Item -LiteralPath $pool[1].FullName -Destination (Join-Path $accDir 'race_humans_starship_02.png')
Copy-Item -LiteralPath $pool[2].FullName -Destination (Join-Path $accDir 'race_humans_starship_03.png')
Copy-Item -LiteralPath $pool[3].FullName -Destination (Join-Path $accDir 'race_humans_cruiser.png')
Copy-Item -LiteralPath $pool[4].FullName -Destination (Join-Path $accDir 'race_humans_cruiser_02.png')
# Конфликт имён: то же имя в игре с ДРУГИМ хэшем — не перезаписывать (и не добавлять запись).
Copy-Item -LiteralPath $pool[5].FullName -Destination (Join-Path $accDir 'race_humans_fighter.png')
Copy-Item -LiteralPath $pool[6].FullName -Destination (Join-Path $sprDir 'race_humans_fighter.png')
# Фильтр: легаси-люди (в мете) и сирота (без записи) импортироваться НЕ должны.
Copy-Item -LiteralPath $pool[6].FullName -Destination (Join-Path $accDir 'race_humans_01.png')
Copy-Item -LiteralPath $pool[5].FullName -Destination (Join-Path $accDir 'race_orphan_99.png')

$meta = @(
    [ordered]@{ file = 'race_humans_starship.png'; race = 'humans'; race_name = 'Humans' },
    [ordered]@{ file = 'race_humans_cruiser_02.png'; race = 'humans'; race_name = 'Humans'; angle = -180; orient_meta = $true },
    [ordered]@{ file = 'race_humans_starship_02.png'; race = 'humans'; race_name = 'Humans'; angle = 20; flip = $true; orient_meta = $true },
    [ordered]@{ file = 'race_humans_cruiser.png'; race = 'humans'; race_name = 'Humans'; angle = 200; orient_meta = $true },
    [ordered]@{ file = 'race_humans_starship_03.png'; race = 'humans'; race_name = 'Humans'; orient_meta = $true },
    [ordered]@{ file = 'race_humans_fighter.png'; race = 'humans'; race_name = 'Humans'; orient_meta = $true },
    [ordered]@{ file = 'race_humans_01.png'; race = 'humans'; race_name = 'Humans' }
)
$utf8nobom = New-Object System.Text.UTF8Encoding($false)
[System.IO.File]::WriteAllText((Join-Path $accDir 'ships_meta.json'), (ConvertTo-Json -InputObject $meta -Depth 5), $utf8nobom)

# JSON-фикстура реестра: людской дефолт (с неизвестным полем scale_human —
# должно сохраниться), нечеловеческая раса (проверяет порядок «прочие после
# людского блока») и нейтральный последним.
$regShips = @(
    [ordered]@{ id = 'race_humans_starship'; name = 'Humans · 1'; file = 'race_humans_starship.png'; race = 'humans'; scale_human = 30 },
    [ordered]@{ id = 'race_coastal_01'; name = 'Coastal · 1'; file = 'race_coastal_01.png'; race = 'coastal' },
    [ordered]@{ id = 'neutral'; name = 'Neutral'; file = 'neutral.png'; race = '' }
)
$regObj = [pscustomobject][ordered]@{ version = 1; ships = @($regShips) }
[System.IO.File]::WriteAllText($regFile, (ConvertTo-Json -InputObject $regObj -Depth 10), $utf8nobom)

$fail = 0
function Check([string]$name, [bool]$cond) {
    if ($cond) { Write-Output ("PASS " + $name) } else { Write-Output ("FAIL " + $name); $script:fail++ }
}

$out1 = (& $import -AcceptedDir $accDir -SpritesDir $sprDir -Registry $regFile | Out-String)
Check 'copies new file starship' (Test-Path -LiteralPath (Join-Path $sprDir 'race_humans_starship.png'))
Check 'copies new file starship_02' (Test-Path -LiteralPath (Join-Path $sprDir 'race_humans_starship_02.png'))
Check 'copies new file starship_03' (Test-Path -LiteralPath (Join-Path $sprDir 'race_humans_starship_03.png'))
Check 'copies new file cruiser' (Test-Path -LiteralPath (Join-Path $sprDir 'race_humans_cruiser.png'))
Check 'copies new file cruiser_02' (Test-Path -LiteralPath (Join-Path $sprDir 'race_humans_cruiser_02.png'))
Check 'conflict name not overwritten' ((Get-FileHash -LiteralPath (Join-Path $sprDir 'race_humans_fighter.png')).Hash -eq (Get-FileHash -LiteralPath $pool[6].FullName).Hash)
Check 'filter: legacy humans not copied' (-not (Test-Path -LiteralPath (Join-Path $sprDir 'race_humans_01.png')))
Check 'filter: orphan not copied' (-not (Test-Path -LiteralPath (Join-Path $sprDir 'race_orphan_99.png')))

# Реестр: читаем JSON-фикстуру после импорта.
$regRaw = Get-Content -LiteralPath $regFile -Raw -Encoding UTF8
$regAfter = ConvertFrom-Json -InputObject $regRaw
$after = @($regAfter.ships)
$byFile = @{}
foreach ($s in $after) { $byFile[[string]$s.file] = $s }

$files = @($after | ForEach-Object { [string]$_.file })
$expectedOrder = @(
    'race_humans_starship.png', 'race_humans_starship_02.png', 'race_humans_starship_03.png',
    'race_humans_cruiser.png', 'race_humans_cruiser_02.png',
    'race_coastal_01.png', 'neutral.png'
)
function Same-Sequence($a, $b) {
    if (@($a).Count -ne @($b).Count) { return $false }
    for ($i = 0; $i -lt @($a).Count; $i++) { if ($a[$i] -ne $b[$i]) { return $false } }
    return $true
}
Check 'canonical order: humans first, races, neutral last' (Same-Sequence $files $expectedOrder)
Check 'new record added (starship_02)' ($byFile.ContainsKey('race_humans_starship_02.png'))
Check 'new record added (cruiser_02)' ($byFile.ContainsKey('race_humans_cruiser_02.png'))
Check 'conflict: no fighter record added' (-not $byFile.ContainsKey('race_humans_fighter.png'))
Check 'existing coastal not duplicated' ((@($after | Where-Object { $_.file -eq 'race_coastal_01.png' })).Count -eq 1)
Check 'scale_human preserved' ($byFile['race_humans_starship.png'].scale_human -eq 30)
Check 'angle 20 flip true' (($byFile['race_humans_starship_02.png'].angle -eq 20) -and ($byFile['race_humans_starship_02.png'].flip -eq $true))
Check 'angle 200 normalized to -160' ($byFile['race_humans_cruiser.png'].angle -eq -160)
Check 'angle -180 normalized to 180' ($byFile['race_humans_cruiser_02.png'].angle -eq 180)
Check 'race written in record' ($byFile['race_humans_cruiser.png'].race -eq 'humans')

# BOM: первые три байта файла не EF BB BF.
$bytes = [System.IO.File]::ReadAllBytes($regFile)
Check 'no BOM' (-not (($bytes.Length -ge 3) -and ($bytes[0] -eq 0xEF) -and ($bytes[1] -eq 0xBB) -and ($bytes[2] -eq 0xBF)))

# Идемпотентность: повторный прогон ничего не добавляет и не копирует.
$snapBefore = (Get-ChildItem -LiteralPath $sprDir -File | ForEach-Object { $_.Name + ':' + (Get-FileHash -LiteralPath $_.FullName).Hash }) -join '|'
$countBefore = $after.Count
$out2 = (& $import -AcceptedDir $accDir -SpritesDir $sprDir -Registry $regFile | Out-String)
$snapAfter = (Get-ChildItem -LiteralPath $sprDir -File | ForEach-Object { $_.Name + ':' + (Get-FileHash -LiteralPath $_.FullName).Hash }) -join '|'
$regAfter2 = ConvertFrom-Json -InputObject (Get-Content -LiteralPath $regFile -Raw -Encoding UTF8)
Check 'idempotent: no new copies' ($snapBefore -eq $snapAfter)
Check 'idempotent: registry size unchanged' (@($regAfter2.ships).Count -eq $countBefore)
Check 'idempotent: nothing added' ($out2 -notmatch 'добавлено в реестр:')

Remove-Item -LiteralPath $tmp -Recurse -Force -ErrorAction SilentlyContinue
Write-Output ('RESULT: ' + $(if ($fail -eq 0) { 'PASS' } else { 'FAIL' }))
exit $(if ($fail -eq 0) { 0 } else { 1 })
