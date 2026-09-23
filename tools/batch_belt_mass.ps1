# batch_belt_mass.ps1 — массовая пачка астероидов пояса (направление 2).
# ТЗ: docs/gamedesign/art/art_belt_asteroids.md §3.3 (этап 1 форма, этап 2 текстура),
# §3.6 (постобработка: вырез по цвету, крупнейший компонент, заливка дырок,
# 256x256 камень / 64x64 обломки, --no-orient).
# 8 форм камня x 2 варианта текстуры (a — матовая пыльная, b — трещиноватая/сколотая)
# + 3 мелких обломка. Промпты/параметры — как в одобренном пилоте.
param(
    [int]$StartSeed = 90001,
    [string]$Stage2SeedBase = "91001",
    [string]$Stage2SeedBaseB = "92001",
    [string]$Only = "",
    [double]$DenoiseA = 0.50,
    [double]$DenoiseB = 0.55
)

. "$PSScriptRoot\comfy_api.ps1"

$neg = "station, vehicle, spaceship, rings, engine, laser, machinery, building, plant, water, snow, ice sheet, text, watermark"

$stage1Core = "single irregular asteroid, rough rocky surface, regolith, craters, fractures, chipped edges, top-down flat view, neutral soft lighting, game asset, 2D sprite, centered, on black background, no text, no watermark"

# Вариант (a) — матовая пыльная: ровнее, светлее.
$stage2a = "ENTIRE asteroid covered with fine dust and regolith, soft matte gray-brown stone, gentle smooth gradients, subtle craters, weathered dusty surface, pale lighter tone, no metal, no machinery, no plants, no fire, maximal detail, masterpiece"
# Вариант (b) — трещиноватая/сколотая: темнее, контрастнее.
$stage2b = "ENTIRE asteroid covered with deep fractures, cracks, chipped edges, broken rugged surface, sharp crevices, dark contrast, heavily cratered stone, no metal, no machinery, no plants, no fire, darker tone, maximal detail, masterpiece"

# Параметры этапов (пилот).
$s1steps = 40; $s1cfg = 7; $s1strength = 1.5; $s1denoise = 0.85
$s2steps = 40; $s2cfg = 7.5
$cannyLow = 0.2; $cannyHigh = 0.5
$model = "juggernaut-xl-v9.safetensors"
$controlnet = "controlnet-canny-sdxl-1.0.safetensors"

# Формы: имя силуэта, размер канвы, паддинг постобработки, вариант-a denoise.
# dA: для мелких/плоских силуэтов вариант a тоже 0.55 — иначе SDXL оставляет
# плоскую «плиту» (грабли массовой пачки); формы с рельефом — 0.50, как в пилоте.
# single: обломки без вариантов (одно имя debris_0N.png, как в §3.7).
$items = @(
    @{ n = "rock_01"; canvas = 256; pad = 5; dA = 0.50 },
    @{ n = "rock_02"; canvas = 256; pad = 5; dA = 0.50 },
    @{ n = "rock_03"; canvas = 256; pad = 5; dA = 0.50 },
    @{ n = "rock_04"; canvas = 256; pad = 5; dA = 0.55 },
    @{ n = "rock_05"; canvas = 256; pad = 5; dA = 0.55 },
    @{ n = "rock_06"; canvas = 256; pad = 5; dA = 0.55 },
    @{ n = "rock_07"; canvas = 256; pad = 5; dA = 0.50 },
    @{ n = "rock_08"; canvas = 256; pad = 5; dA = 0.55 },
    @{ n = "debris_01"; canvas = 64; pad = 3; dA = 0.52; single = $true },
    @{ n = "debris_02"; canvas = 64; pad = 3; dA = 0.52; single = $true },
    @{ n = "debris_03"; canvas = 64; pad = 3; dA = 0.52; single = $true }
)

# Сиды силуэтов (make_belt_silhouettes.py).
$silSeeds = @{
    "rock_01" = 20260923; "rock_02" = 20260926; "rock_03" = 20260925
    "rock_04" = 20260927; "rock_05" = 20260928; "rock_06" = 20260929
    "rock_07" = 20260930; "rock_08" = 20260931
    "debris_01" = 20260932; "debris_02" = 20260933; "debris_03" = 20260934
}

$silDir  = "C:\Zorion2\ai_drafts\belt_asteroids\silhouettes"
$s1Dir   = "C:\Zorion2\ai_drafts\belt_asteroids\stage1"
$s2Dir   = "C:\Zorion2\ai_drafts\belt_asteroids\stage2"
$procDir = "C:\Zorion2\ai_drafts\belt_asteroids\sprites"
New-Item -ItemType Directory -Path $s1Dir, $s2Dir, $procDir -Force | Out-Null

$py = "C:\ComfyUI\venv\Scripts\python.exe"
$sprites = @()
$onlyList = @()
if ($Only) { $onlyList = @($Only -split ',' | ForEach-Object { $_.Trim() } | Where-Object { $_ }) }

for ($i = 0; $i -lt $items.Count; $i++) {
    $body = $items[$i].n
    if ($onlyList.Count -gt 0 -and ($onlyList -notcontains $body)) { continue }
    $canvas = $items[$i].canvas
    $pad = $items[$i].pad
    $dA = $items[$i].dA
    if ($null -eq $dA) { $dA = $DenoiseA }
    if ($items[$i].single) { $dA = $items[$i].dA }
    $seed1 = $StartSeed + $i
    $seedA = [int]$Stage2SeedBase + $i
    $seedB = [int]$Stage2SeedBaseB + $i
    $sil = "$silDir\$body.png"
    $st1 = "$s1Dir\${body}_stage1.png"

    Write-Host "=== $($i+1)/$($items.Count) [$body] canvas=$canvas stage1 seed=$seed1, A=$seedA/$dA, B=$seedB ==="

    $r1 = Send-ComfyControlNet -Prompt $stage1Core -SilhouettePath $sil -NegativePrompt $neg `
        -Seed $seed1 -Steps $s1steps -Cfg $s1cfg -Strength $s1strength -Denoise $s1denoise -OutputPath $st1
    if (-not $r1) { Write-Host "Stage1 FAILED $body"; continue }

    if ($items[$i].single) {
        $variants = @(@{k = ""; p = $stage2a; s = $seedA; d = $dA})
    } else {
        $variants = @(@{k = "a"; p = $stage2a; s = $seedA; d = $dA},
                      @{k = "b"; p = $stage2b; s = $seedB; d = $DenoiseB})
    }

    foreach ($v in $variants) {
        $fin = "$s2Dir\${body}$($v.k).png"
        $clean = "$procDir\${body}$($v.k).png"
        $r2 = Send-ComfyImg2Img -Prompt $v.p -InputImage $st1 -NegativePrompt $neg `
            -Seed $v.s -Steps $s2steps -Cfg $s2cfg -Denoise $v.d -OutputPath $fin
        if (-not $r2) { Write-Host "Stage2 FAILED $body$($v.k)"; continue }
        & $py "C:\Zorion2\tools\process_belt_rock.py" $fin $clean --canvas $canvas --pad $pad
        $sprites += [pscustomobject]@{
            name = "$body$($v.k)"
            silhouette = "$body.png"
            silhouette_seed = $silSeeds[$body]
            variant = $(if ($v.k) { $v.k } else { "single" })
            stage1_seed = $seed1
            stage2_seed = $v.s
            stage2_denoise = $v.d
            canvas = $canvas
        }
    }
}

Write-Host "=== Done: mass belt asteroids ($($sprites.Count) sprites) ==="

if ($onlyList.Count -gt 0) {
    Write-Host "Partial run (-Only): manifest not written"
    return
}

$manifest = [ordered]@{
    batch = "belt_asteroids_mass"
    date = "2026-09-23"
    model = $model
    controlnet = $controlnet
    technique = [ordered]@{
        silhouette = "1024x1024, чёрный фон (5,5,5), одно тело ~80% холста, тоновая подложка (светлая кромка сверху, основа #6b7280, тени #2b303a)"
        craters = "Вмятины/крáтеры — ТОЛЬКО тонкая тёмная дуга-кромка БЕЗ заливки и без светлого кольца. Заливка/кольцо заставляют SDXL лепить крáтер выпуклым шаром/валуном (находка пилота)."
        post = "process_belt_rock.py: вырез фона по цвету, крупнейший компонент, ЗАЛИВКА ВНУТРЕННИХ ДЫР (тёмные трещины так же черны, как фон), сглаживание, паддинг, 256 (камень) / 64 (обломки), --no-orient, без метаданных поворота"
    }
    prompts = [ordered]@{
        stage1 = $stage1Core; stage2a = $stage2a; stage2b = $stage2b; negative = $neg
    }
    params = [ordered]@{
        stage1 = @{ steps = $s1steps; cfg = $s1cfg; strength = $s1strength; denoise = $s1denoise; sampler = "dpmpp_2m"; scheduler = "karras"; canny_low = $cannyLow; canny_high = $cannyHigh }
        stage2 = @{ steps = $s2steps; cfg = $s2cfg; denoise_a = $DenoiseA; denoise_b = $DenoiseB; sampler = "dpmpp_2m"; scheduler = "karras" }
    }
    sprites = $sprites
}
$json = $manifest | ConvertTo-Json -Depth 8
[System.IO.File]::WriteAllText("C:\Zorion2\ai_drafts\belt_asteroids\manifest.json", $json,
    (New-Object System.Text.UTF8Encoding($false)))
Write-Host "Manifest: C:\Zorion2\ai_drafts\belt_asteroids\manifest.json"
