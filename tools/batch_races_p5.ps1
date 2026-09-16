# batch_races_p5.ps1 — пачка 5: КАРДИНАЛЬНО РАЗНЫЕ ПОДХОДЫ (нащупываем стиль).
# Строгое правило: низ «посажен» (шея/туловище уходят за нижний край).
# ВАЖНО (инсайт из гайда ControlNet): варьируем Strength (жёсткость силуэта) и CNEnd
# (доля шагов, где силуэт влияет) — 0.8-1.5 / 0.3-0.6.
# 6 ControlNet-подходов + 2 txt2img (живописный, макро).
param([int]$StartSeed = 70000)

. "$PSScriptRoot\comfy_api.ps1"

$outDir = "C:\Zorion2\ai_drafts\races_p5"
$procDir = "C:\Zorion2\ai_drafts\races_p5_processed"
New-Item -ItemType Directory -Path $outDir -Force | Out-Null
New-Item -ItemType Directory -Path $procDir -Force | Out-Null

$py = "C:\ComfyUI\venv\Scripts\python.exe"
$log = "$outDir\log.txt"
Add-Content $log "=== races_p5 start $(Get-Date -Format s) ==="

# --- txt2img: ЖИВОПИСНЫЙ (F4 Курильщик) и МАКРО (F2 Лёд) ---
$negArt = "text, watermark, blurry, low quality, deformed, ugly, duplicate, extra fingers, side view, profile, 3D render, cartoon, anime, flat, plain"

$artPrompts = @(
    "dramatic cinematic concept art portrait of an alien sulfur creature, FRONT VIEW, massive basalt rock head with curved horns, glowing amber slit eyes, chiaroscuro lighting, volumetric smoke, deep shadows, painterly brushwork, head and shoulders, torso extending down below the frame, anchored, centered, on black background, masterpiece, game avatar, no text, no watermark",
    "extreme macro close-up of alien cryo-ammonia life, FRONT VIEW, crystalline ice formation resembling a creature head, translucent pale blue ice facets, intricate frost texture, glowing ice core, microscopic detail, head and shoulders, torso extending down below the frame, anchored, centered, on black background, masterpiece, game avatar, no text, no watermark"
)

for ($i = 0; $i -lt 2; $i++) {
    $seed = $StartSeed + $i
    $num = $i + 1
    Write-Host "=== $num/8 [txt2img $($i+1)] seed=$seed ==="
    Add-Content $log "--- $num/8 txt2img seed=$seed"
    $fin = "$outDir\v${num}.png"
    $r = Send-ComfyPrompt -Prompt $artPrompts[$i] -NegativePrompt $negArt -Width 1024 -Height 1024 -Steps 40 -Cfg 7 -Seed $seed -Model "juggernaut-xl-v9.safetensors" -OutputPath $fin
    if (-not $r) { Add-Content $log "art v$num failed"; continue }
    & $py "C:\Zorion2\tools\process_ship.py" $fin "$procDir\v${num}.png" --no-orient
    Add-Content $log "OK v$num"
}

# --- ControlNet-подходы: у каждого СВОИ Strength/CNEnd ---
$stage1 = @{
    "organic_head" = "organic alien creature HEAD ONLY, FRONT VIEW, soft wet amphibian head with membranes and tentacles, bioluminescent glow, smooth slimy skin, drooping folds, head and shoulders, torso extending down below the frame, anchored, not floating, centered, single creature head, on black background, game avatar, no text, no watermark"
    "techno_head" = "cyberpunk alien machine head, FRONT VIEW, industrial metal skull with pipes and gears, glowing lava veins in steel, circular lamp eyes, head and shoulders, torso extending down below the frame, anchored, not floating, centered, single creature head, on black background, game avatar, no text, no watermark"
    "material_head" = "alien head made of MOLTEN GLASS and obsidian, FRONT VIEW, translucent glass drop head with gold veins inside, dark empty eye sockets, glowing core, liquid glass drips, head and shoulders, torso extending down below the frame, anchored, not floating, centered, single creature head, on black background, game avatar, no text, no watermark"
    "geometry_head" = "abstract geometric alien energy entity HEAD ONLY, FRONT VIEW, pure geometric form, neon triangle frame around glowing violet sphere, concentric energy rings, minimal, symmetrical, torso extending down below the frame, anchored, centered, single entity, on black background, game avatar, no text, no watermark"
    "emblem_head" = "heraldic emblem portrait of an alien fungus race, FRONT VIEW, symmetrical ceremonial shield with mushroom cap, golden mantle collar, ornamental, regal, head and shoulders, torso extending down below the frame, anchored, centered, on black background, game avatar, no text, no watermark"
    "character_head" = "fierce aggressive alien creature HEAD ONLY, FRONT VIEW, snarling stone head with wide open vent mouth full of teeth, burning amber eyes, heavy brow ridges, angry expression, head and shoulders, torso extending down below the frame, anchored, not floating, centered, single creature head, on black background, game avatar, no text, no watermark"
}

$stage2 = "realistic alien creature head close-up, FRONT VIEW, ENTIRE head covered with detailed alien texture, scales, pores, cracks, glowing details, subtle gradients, rich palette, maximal detail, masterpiece, game avatar, centered, single creature head, torso extending down below the frame, anchored, on black background, no text, no watermark"

$silMap = @{
    "organic_head" = "sil_race_organic_head.png"
    "techno_head" = "sil_race_techno_head.png"
    "material_head" = "sil_race_material_head.png"
    "geometry_head" = "sil_race_geometry_head.png"
    "emblem_head" = "sil_race_emblem_head.png"
    "character_head" = "sil_race_character_head.png"
}

# Strength/CNEnd эксперимент: (F1 organic 1.2/0.4, F5 techno 1.0/0.4, F6 material 0.9/0.3,
# F9 geometry 1.3/0.6, F3 emblem 1.4/0.5, F8 character 1.1/0.4)
$params = @{
    "organic_head" = @(1.2, 0.4); "techno_head" = @(1.0, 0.4)
    "material_head" = @(0.9, 0.3); "geometry_head" = @(1.3, 0.6)
    "emblem_head" = @(1.4, 0.5); "character_head" = @(1.1, 0.4)
}

$concepts = @($silMap.Keys)
for ($i = 0; $i -lt 6; $i++) {
    $concept = $concepts[$i]
    $seed = $StartSeed + 100 + $i
    $num = $i + 3
    $strength = $params[$concept][0]
    $cnEnd = $params[$concept][1]
    $silPath = "C:\Zorion2\ai_drafts\silhouettes_races\$($silMap[$concept])"
    Write-Host "=== $num/8 [$concept] seed=$seed strength=$strength cnEnd=$cnEnd ==="
    Add-Content $log "--- $num/8 [$concept] seed=$seed str=$strength cnEnd=$cnEnd"
    $s1 = "$outDir\v${num}_stage1.png"
    $fin = "$outDir\v${num}.png"

    $r1 = Send-ComfyControlNet -Prompt $stage1[$concept] -SilhouettePath $silPath -Seed $seed -Steps 40 -Cfg 7 -Strength $strength -Denoise 0.85 -CNEnd $cnEnd -OutputPath $s1
    if (-not $r1) { Add-Content $log "Stage1 failed v$num"; continue }
    $r2 = Send-ComfyImg2Img -Prompt $stage2 -InputImage $s1 -Seed ($seed + 500) -Steps 40 -Cfg 7.5 -Denoise 0.55 -OutputPath $fin
    if (-not $r2) { Add-Content $log "Stage2 failed v$num"; continue }
    & $py "C:\Zorion2\tools\process_ship.py" $fin "$procDir\v${num}.png" --no-orient
    Add-Content $log "OK v$num"
}

Add-Content $log "=== races_p5 done $(Get-Date -Format s) ==="
Write-Host "=== Done: 8 avatars (pack 5: 8 approaches) ==="