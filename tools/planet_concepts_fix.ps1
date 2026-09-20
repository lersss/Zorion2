# planet_concepts_fix.ps1 — правка референсов А/Б/В по замечанию создателя:
# «странный фон вокруг диска планеты». Перегенерация img2img от текущих файлов
# (сохранить расклад биомов, denoise 0.6-0.65) + гарантированная пост-чистка фона
# (planet_cleanbg.py: ровный тёмный космос + редкие мелкие звёзды, без пятен/ореолов).
# Планета в игре вставляется на канвас — вокруг диска должен быть чистый космос.
param(
    [int]$Stage = 1,
    [int]$Seed = -1
)

. "$PSScriptRoot\comfy_api.ps1"

if (-not (Test-ComfyReady)) { Write-Error "ComfyUI is down"; exit 1 }

$outDir = "C:\Zorion2\ai_drafts\planet_concepts"
$py = "C:\ComfyUI\venv\Scripts\python.exe"
New-Item -ItemType Directory -Path $outDir -Force | Out-Null

# Общий каркас (в начало каждого промпта) — перевод ТЗ
$common = "View of a planet from orbit, full disk centered in a square frame, single light source - star at upper left, soft terminator, separate pieces of surface (biomes) are distinguishable, atmospheric haze along the edge of the disk and cloud cover, no text, no ships, no UI, no frames, high resolution"

# Общее описание планеты (одинаковое во всех стилях — одна и та же планета)
$planet = "Earth-like planet: deep blue oceans, green continents with forests, ochre deserts, white polar ice caps with ragged edges"

# ФРАЗА ПРО ЧИСТЫЙ КОСМОС — главное изменение: диск в центре, вокруг ровный чёрный
# космос с редкими мелкими звёздами, ничего лишнего (никаких небул, ореолов, пятен,
# «окон», рамок). Диск ~70-80% кадра.
$cleanSpace = "The planet is a single clean disk in the CENTER of the frame, the disk diameter is about 75 percent of the frame width. Around the disk: PLAIN BLACK SPACE with only a few subtle tiny stars, no nebula, no clouds in space, no glow, no halo, no gradient, no vignette, no window, no screen, no frame, no artifacts. Clean solid dark background"

$negBase = "text, watermark, blurry, low quality, deformed, ugly, duplicate, extra fingers, UI, frame, border, half planet, crop, close-up, zoomed in, planet filling the whole frame, no space background"
$negSpace = "nebula, glowing gas cloud, lens flare, halo around planet, gradient background, vignette, smudges, artifacts, window, screen, monitor, glass reflection, bokeh, light rays, lens dust"
$negReal = "$negBase, $negSpace"
$negFlat = "$negBase, $negSpace, photorealistic, photo, 3D render, texture, noise, grain, gradient, shading, depth"
$negArt  = "$negBase, $negSpace"

# --- Стиль А: Орбитальная съёмка (реализм) ---
$aPrompt = "$common. Realistic orbital photograph of a planet. $cleanSpace. $planet. Thin bluish atmospheric haze glows ONLY along the very edge of the disk, translucent white clouds swirl in bands over land and ocean. Soft terminator, light white glare from the star. Photorealism, smooth transitions between landscapes"

# --- Стиль Б: Флэт-карта (графическая стилизация) ---
$bPrompt = "$common. Flat graphic illustration of a planet in strategy map style. $cleanSpace. Sharp borders between colored regions: green plains, blue oceans, yellow deserts, white polar caps, dark red volcanic areas. Saturated flat clean colors, no textures or gradients on the surface. Simple dark terminator, thin flat atmosphere ring, white cloud patches with even edges. Minimalism, vector look"

# --- Стиль В: Концепт-арт (живописная графика) ---
$cPrompt = "$common. Painterly concept art of a planet from orbit. $cleanSpace. Dramatic contrasty lighting, warm and cold zones clash at the terminator. Rich oceans with turquoise highlights, amber-green continents, glowing orange lava cracks on dark crust, bluish polar caps. Thin multi-layered limb glow of the atmosphere ONLY at the disk edge, voluminous clouds with soft shadows, large warm star glare. Painterly graphics, brushstroke texture"

$baseFile = "$outDir\A_orbital.png"
$bFile = "$outDir\B_flat.png"
$cFile = "$outDir\C_concept.png"

function Invoke-CleanBg([string]$path) {
    Write-Host "  cleanbg: $path"
    $tmp = "$path.tmp.png"
    & $py "C:\Zorion2\tools\planet_cleanbg.py" $path $tmp 2>&1 | ForEach-Object { Write-Host "    $_" }
    if (-not (Test-Path $tmp)) { Write-Error "cleanbg failed for $path"; return $false }
    Move-Item $tmp $path -Force
    return $true
}

if ($Stage -le 1) {
    Write-Host "=== Stage 1: A (orbital) from current A, denoise 0.6 + cleanbg ==="
    $oldA = "$outDir\A_orbital.png"
    if (-not (Test-Path $oldA)) { Write-Error "No $oldA"; exit 1 }
    $r = Send-ComfyImg2Img -Prompt $aPrompt -NegativePrompt $negReal -InputImage $oldA -Seed $Seed -Steps 32 -Cfg 7.0 -Denoise 0.6 -Model "juggernaut-xl-v9.safetensors" -OutputPath "$outDir\A_fix_raw.png"
    if (-not $r) { Write-Error "Stage 1 A failed"; exit 1 }
    Copy-Item "$outDir\A_fix_raw.png" $baseFile -Force
    if (-not (Invoke-CleanBg $baseFile)) { exit 1 }
}

if ($Stage -ge 2) {
    if (-not (Test-Path $baseFile)) { Write-Error "No base $baseFile - run -Stage 1 first"; exit 1 }
    Write-Host "=== Stage 2: B (flat) and C (concept art) from new A + cleanbg ==="
    $rb = Send-ComfyImg2Img -Prompt $bPrompt -NegativePrompt $negFlat -InputImage $baseFile -Steps 32 -Cfg 7.0 -Denoise 0.65 -Seed $Seed -Model "juggernaut-xl-v9.safetensors" -OutputPath "$outDir\B_fix_raw.png"
    if (-not $rb) { Write-Error "Stage 2 B failed"; exit 1 }
    Copy-Item "$outDir\B_fix_raw.png" $bFile -Force
    if (-not (Invoke-CleanBg $bFile)) { exit 1 }

    $rc = Send-ComfyImg2Img -Prompt $cPrompt -NegativePrompt $negArt -InputImage $baseFile -Steps 32 -Cfg 7.0 -Denoise 0.6 -Seed $Seed -Model "juggernaut-xl-v9.safetensors" -OutputPath "$outDir\C_fix_raw.png"
    if (-not $rc) { Write-Error "Stage 2 C failed"; exit 1 }
    Copy-Item "$outDir\C_fix_raw.png" $cFile -Force
    if (-not (Invoke-CleanBg $cFile)) { exit 1 }
}

Write-Host "=== Done ==="
Get-ChildItem $outDir -Filter "*_fix*" | Select-Object Name, Length | Format-Table -AutoSize