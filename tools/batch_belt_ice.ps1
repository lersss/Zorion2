# batch_belt_ice.ps1 — ледяная пачка астероидов пояса (второй ресурс, этап 4c).
# ТЗ: docs/gamedesign/art/art_belt_asteroids.md §10.4–§10.7.
# Отличия от batch_belt_mass.ps1:
#   * из негатива убраны snow, ice sheet, water (иначе SDXL получает запрет на лёд),
#     добавлены fire, lava, rust, amber, warm orange light, smooth sphere, cut diamond;
#   * два варианта текстуры: a — плотный лёд, b — сколотый лёд;
#   * свои сиды (93xxx силуэты, 931xx этап 1, 941xx/942xx этап 2, 943xx обломки);
#   * свой манифест (ice/manifest.json), финальные PNG — в web/static/sprites/belt/ice_*.
# 8 форм × 2 варианта (ice_01a…ice_08b) + 3 обломка = 19 PNG; process_belt_rock.py
# используется без правок (256 камень / 64 обломки, без поворота).
param(
    [int]$StartSeed = 93101,
    [string]$Stage2SeedBase = "94101",
    [string]$Stage2SeedBaseB = "94201",
    [string]$Only = "",
    [double]$DenoiseA = 0.50,
    [double]$DenoiseB = 0.55
)

. "$PSScriptRoot\comfy_api.ps1"

$neg = "station, vehicle, spaceship, rings, engine, laser, machinery, building, plant, fire, lava, rust, amber, warm orange light, metal, smooth sphere, snowman, snowball, cut diamond, jewelry, gemstone facets grid, text, watermark"

$stage1Core = "single irregular ice asteroid, translucent frozen water ice, angular faceted shards, sharp chipped edges, internal fractures, glassy crystalline surface, top-down flat view, cold blue-white lighting, game asset, 2D sprite, centered, on black background, no text, no watermark"

# Вариант (a) — плотный лёд: ровнее, сильнее внутреннее свечение, меньше трещин.
$stage2a = "ENTIRE ice asteroid of dense translucent blue-white water ice, smooth glassy facets, soft internal glow, faint bubbles and hairline fractures, pale frozen surface, cold cyan-blue tones, no metal, no machinery, no plants, no fire, no lava, maximal detail, masterpiece"
# Вариант (b) — сколотый лёд: гранёный, трещины и сколы, контрастнее.
$stage2b = "ENTIRE ice asteroid of fractured shattered ice, sharp angular facets, chipped broken edges, deep crevices, internal cracks, cleavage planes, milky translucent ice, cold blue-white contrast, no metal, no machinery, no plants, no fire, no lava, maximal detail, masterpiece"

# Параметры этапов (те же, что у камня).
$s1steps = 40; $s1cfg = 7; $s1strength = 1.5; $s1denoise = 0.85
$s2steps = 40; $s2cfg = 7.5
$cannyLow = 0.2; $cannyHigh = 0.5
$model = "juggernaut-xl-v9.safetensors"
$controlnet = "controlnet-canny-sdxl-1.0.safetensors"

$items = @(
    @{ n = "ice_01"; canvas = 256; pad = 5; dA = 0.55 },
    @{ n = "ice_02"; canvas = 256; pad = 5; dA = 0.55 },
    @{ n = "ice_03"; canvas = 256; pad = 5; dA = 0.55 },
    @{ n = "ice_04"; canvas = 256; pad = 5; dA = 0.52 },
    @{ n = "ice_05"; canvas = 256; pad = 5; dA = 0.55 },
    @{ n = "ice_06"; canvas = 256; pad = 5; dA = 0.53 },
    @{ n = "ice_07"; canvas = 256; pad = 5; dA = 0.55 },
    @{ n = "ice_08"; canvas = 256; pad = 5; dA = 0.55 },
    @{ n = "ice_debris_01"; canvas = 64; pad = 3; dA = 0.52; single = $true },
    @{ n = "ice_debris_02"; canvas = 64; pad = 3; dA = 0.52; single = $true },
    @{ n = "ice_debris_03"; canvas = 64; pad = 3; dA = 0.52; single = $true }
)

# Сиды силуэтов (make_belt_silhouettes.py, §10.4).
$silSeeds = @{
    "ice_01" = 93001; "ice_02" = 93002; "ice_03" = 93003; "ice_04" = 93004
    "ice_05" = 93005; "ice_06" = 93006; "ice_07" = 93007; "ice_08" = 93008
    "ice_debris_01" = 93011; "ice_debris_02" = 93012; "ice_debris_03" = 93013
}

$silDir  = "C:\Zorion2\ai_drafts\belt_asteroids\ice\silhouettes"
$s1Dir   = "C:\Zorion2\ai_drafts\belt_asteroids\ice\stage1"
$s2Dir   = "C:\Zorion2\ai_drafts\belt_asteroids\ice\stage2"
$procDir = "C:\Zorion2\ai_drafts\belt_asteroids\ice\sprites"
$webDir  = "C:\Zorion2\web\static\sprites\belt"
New-Item -ItemType Directory -Path $s1Dir, $s2Dir, $procDir, $webDir -Force | Out-Null

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
    $seed1 = $StartSeed + $i
    if ($items[$i].single) { $seedA = 94301 + $i } else { $seedA = [int]$Stage2SeedBase + $i }
    $seedB = [int]$Stage2SeedBaseB + $i
    $sil = "$silDir\$body.png"
    $st1 = "$s1Dir\${body}_stage1.png"

    Write-Host "=== $($i+1)/$($items.Count) [$body] canvas=$canvas stage1 seed=$seed1, A=$seedA/$dA, B=$seedB ==="

    if (-not (Test-Path $sil)) { Write-Host "Silhouette missing: $sil"; continue }

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
        if (Test-Path $clean) { Copy-Item $clean "$webDir\${body}$($v.k).png" -Force }
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

Write-Host "=== Done: belt ice ($($sprites.Count) sprites) ==="

if ($onlyList.Count -gt 0) {
    Write-Host "Partial run (-Only): manifest not written"
    return
}

$manifest = [ordered]@{
    batch = "belt_asteroids_ice"
    date = "2026-09-24"
    model = $model
    controlnet = $controlnet
    technique = [ordered]@{
        silhouette = "1024x1024, чёрный фон (5,5,5), одно тело ~80% холста, ХОЛОДНАЯ тоновая подложка (кромка #eaf4ff, основа #a9c6dc, грань #b3c6d6, трещины #3b414b), ядро #f0f8ff; гранёные формы (facet/chip/plate), кливаж параллельными плоскостями"
        negative = "из негатива камня УБРАНЫ snow/ice sheet/water; добавлены fire, lava, rust, amber, warm orange light, metal, smooth sphere, cut diamond (§10.5)"
        post = "process_belt_rock.py без правок: вырез фона по цвету, крупнейший компонент, заливка внутренних дыр, сглаживание, паддинг, 256 (камень) / 64 (обломки), без нормализации поворота"
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
[System.IO.File]::WriteAllText("C:\Zorion2\ai_drafts\belt_asteroids\ice\manifest.json", $json,
    (New-Object System.Text.UTF8Encoding($false)))
Write-Host "Manifest: C:\Zorion2\ai_drafts\belt_asteroids\ice\manifest.json"
