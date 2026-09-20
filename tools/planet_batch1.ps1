# planet_batch1.ps1 — 3 кандидата базы А (txt2img, разные сиды), выбор лучшего по
# позиции диска и биомам, композиция в центр, img2img-доработка.
param(
    [int]$Stage = 1
)

. "$PSScriptRoot\comfy_api.ps1"

if (-not (Test-ComfyReady)) { Write-Error "ComfyUI is down"; exit 1 }

$outDir = "C:\Zorion2\ai_drafts\planet_concepts"
$py = "C:\ComfyUI\venv\Scripts\python.exe"
New-Item -ItemType Directory -Path $outDir -Force | Out-Null

$common = "View of a planet from orbit, full disk centered in a square frame, single light source - star at upper left, soft terminator, separate pieces of surface (biomes) are distinguishable, atmospheric haze along the edge of the disk and cloud cover, no text, no ships, no UI, no frames, high resolution"
$planet = "Earth-like planet: deep blue oceans, green continents with forests, ochre deserts, white polar ice caps with ragged edges"
$negReal = "text, watermark, blurry, low quality, deformed, ugly, duplicate, extra fingers, UI, frame, border, half planet, crop, close-up, zoomed in, planet filling the whole frame, no space background"

$aPrompt = "$common. Realistic orbital photograph of a planet. The planet is a small disk in the CENTER of the frame, the disk diameter is about 55 percent of the frame width, surrounded by plenty of black space. $planet. Thin bluish atmospheric haze glows all around the disk edge, translucent white clouds swirl in bands over land and ocean. Soft terminator, light white glare from the star. Photorealism, smooth transitions between landscapes, black space around the planet"

$bestScore = -1
$bestFile = ""
$bestInfo = ""

for ($i = 0; $i -lt 3; $i++) {
    $seed = 20260920 + $i * 777
    $cand = "$outDir\A_cand$i.png"
    Write-Host "=== Candidate $i (seed=$seed) ==="
    $r = Send-ComfyPrompt -Prompt $aPrompt -NegativePrompt $negReal -Width 1024 -Height 1024 -Steps 32 -Cfg 7.0 -Seed $seed -Model "juggernaut-xl-v9.safetensors" -OutputPath $cand
    if (-not $r) { Write-Host "cand $i failed"; continue }
    # Анализ: позиция диска, радиус, палитра
    $json = & $py "C:\Zorion2\tools\planet_score.py" $cand
    $score = ($json -split ' ')[0]
    $info = ($json -split ' ', 2)[1]
    Write-Host "score=$score info=$info"
    if ([double]$score -gt $bestScore) {
        $bestScore = [double]$score
        $bestFile = $cand
        $bestInfo = $info
    }
}

if (-not $bestFile) { Write-Error "no good candidate"; exit 1 }
Write-Host "Best: $bestFile (score=$bestScore, $bestInfo)"

# Композиция лучшего в центр
$composed = "$outDir\A_composed.png"
& $py "C:\Zorion2\tools\planet_compose.py" $bestFile $composed 290

# Доработка (детализация, img2img от композиции)
$baseFile = "$outDir\A_orbital.png"
$r2 = Send-ComfyImg2Img -Prompt $aPrompt -NegativePrompt $negReal -InputImage $composed -Seed 20260920 -Steps 32 -Cfg 7.0 -Denoise 0.45 -Model "juggernaut-xl-v9.safetensors" -OutputPath $baseFile
if (-not $r2) { Write-Error "detail pass failed"; exit 1 }
Write-Host "=== Done: $baseFile ==="