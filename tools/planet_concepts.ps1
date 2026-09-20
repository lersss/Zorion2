# planet_concepts.ps1 — 3 референса стилистики планеты (вид с орбиты) для выбора создателем.
# А «Орбитальная съёмка» (база, txt2img) -> Б «Флэт-карта» и В «Концепт-арт» (img2img от А,
# чтобы расклад континентов/океанов совпадал — разница только в стиле).
# Этапы: -Stage 1 (база А) -> -Stage 2 (Б и В из А).
# Промпты — перевод ТЗ дизайнера на английский (Juggernaut XL не понимает кириллицу).
param(
    [int]$Stage = 1,
    [int]$Seed = -1
)

. "$PSScriptRoot\comfy_api.ps1"

if (-not (Test-ComfyReady)) { Write-Error "ComfyUI is down"; exit 1 }

$outDir = "C:\Zorion2\ai_drafts\planet_concepts"
New-Item -ItemType Directory -Path $outDir -Force | Out-Null

# Общий каркас (в начало каждого промпта) — перевод ТЗ
$common = "View of a planet from orbit, full disk centered in a square frame, single light source - star at upper left, soft terminator, separate pieces of surface (biomes) are distinguishable, atmospheric haze along the edge of the disk and cloud cover, no text, no ships, no UI, no frames, high resolution"

# Общее описание планеты (одинаковое во всех стилях — одна и та же планета)
$planet = "Earth-like planet: deep blue oceans, green continents with forests, ochre deserts, white polar ice caps with ragged edges"

$negReal = "text, watermark, blurry, low quality, deformed, ugly, duplicate, extra fingers, UI, frame, border, half planet, crop, close-up, zoomed in, planet filling the whole frame, no space background"
$negFlat = "text, watermark, blurry, low quality, deformed, ugly, duplicate, photorealistic, photo, 3D render, texture, noise, grain, gradient, shading, depth, UI, frame, border, half planet, planet filling the whole frame"
$negArt  = "text, watermark, blurry, low quality, deformed, ugly, duplicate, UI, frame, border, half planet, crop, planet filling the whole frame"

# --- Стиль А: Орбитальная съёмка (реализм) — база ---
$aPrompt = "$common. Realistic orbital photograph of a planet. The planet is a small disk in the CENTER of the frame, the disk diameter is about 55 percent of the frame width, surrounded by plenty of black space. $planet. Thin bluish atmospheric haze glows all around the disk edge, translucent white clouds swirl in bands over land and ocean. Soft terminator, light white glare from the star. Photorealism, smooth transitions between landscapes, black space around the planet"

# --- Стиль Б: Флэт-карта (графическая стилизация) ---
$bPrompt = "$common. Flat graphic illustration of a planet in strategy map style. Sharp borders between colored regions: green plains, blue oceans, yellow deserts, white polar caps, dark red volcanic areas. Saturated flat clean colors, no textures or gradients on the surface. Simple dark terminator, thin flat glowing atmosphere ring, white cloud patches with even edges. Minimalism, vector look, black space around the planet"

# --- Стиль В: Концепт-арт (живописная графика) ---
$cPrompt = "$common. Painterly concept art of a planet from orbit. Dramatic contrasty lighting, warm and cold zones clash at the terminator. Rich oceans with turquoise highlights, amber-green continents, glowing orange lava cracks on dark crust, bluish polar caps. Bright multi-layered limb glow of the atmosphere, voluminous clouds with soft shadows, large warm star glare. Painterly graphics, brushstroke texture, black space around the planet"

$baseFile = "$outDir\A_orbital.png"
$bFile = "$outDir\B_flat.png"
$cFile = "$outDir\C_concept.png"

if ($Stage -le 1) {
    Write-Host "=== Stage 1: base A - compose cand2 + detail pass ==="
    $py = "C:\ComfyUI\venv\Scripts\python.exe"
    $composed = "$outDir\A_composed2.png"
    if (-not (Test-Path $composed)) { Write-Error "No $composed - compose from A_cand2 first"; exit 1 }
    $r = Send-ComfyImg2Img -Prompt $aPrompt -NegativePrompt $negReal -InputImage $composed -Seed $Seed -Steps 32 -Cfg 7.0 -Denoise 0.35 -Model "juggernaut-xl-v9.safetensors" -OutputPath $baseFile
    if (-not $r) { Write-Error "Stage 1 failed"; exit 1 }
}

if ($Stage -ge 2) {
    if (-not (Test-Path $baseFile)) { Write-Error "No base $baseFile - run -Stage 1 first"; exit 1 }
    Write-Host "=== Stage 2: B (flat) and C (concept art) from base A ==="
    $rb = Send-ComfyImg2Img -Prompt $bPrompt -NegativePrompt $negFlat -InputImage $baseFile -Steps 32 -Cfg 7.0 -Denoise 0.65 -Seed $Seed -Model "juggernaut-xl-v9.safetensors" -OutputPath $bFile
    if (-not $rb) { Write-Error "Stage 2 B failed"; exit 1 }
    $rc = Send-ComfyImg2Img -Prompt $cPrompt -NegativePrompt $negArt -InputImage $baseFile -Steps 32 -Cfg 7.0 -Denoise 0.6 -Seed $Seed -Model "juggernaut-xl-v9.safetensors" -OutputPath $cFile
    if (-not $rc) { Write-Error "Stage 2 C failed"; exit 1 }
}

Write-Host "=== Done ==="
Get-ChildItem $outDir | Select-Object Name, Length | Format-Table -AutoSize