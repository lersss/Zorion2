# batch_races_p6v.ps1 — пачка 6 вилка: ЖИВОПИСНЫЙ стиль × антропоморфный/неантропоморфный.
# Пилот: 4 семейства (F1, F4, F6, F9), каждое в 2 вилках = 8 аватаров. txt2img, без силуэта.
# Пост: process_ship --no-orient. Проверка: caption_image.py.
param([int]$StartSeed = 82000)

. "$PSScriptRoot\comfy_api.ps1"

$outDir = "C:\Zorion2\ai_drafts\races_p6"
$procDir = "C:\Zorion2\ai_drafts\races_p6_processed"
New-Item -ItemType Directory -Path $outDir -Force | Out-Null
New-Item -ItemType Directory -Path $procDir -Force | Out-Null

$py = "C:\ComfyUI\venv\Scripts\python.exe"
$log = "$outDir\log_v.txt"
Add-Content $log "=== races_p6_vfork start $(Get-Date -Format s) ==="

$styleTail = "head and shoulders, torso extending down below the frame, anchored, centered, on black background, masterpiece, game avatar, no text, no watermark"
$negArt = "text, watermark, blurry, low quality, deformed, ugly, duplicate, extra fingers, side view, profile, three-quarter view, 3D render, cartoon, anime, flat, plain, floating, cut off head, no body, disembodied"

# 8 промптов: 4 семейства x 2 вилки (антропо/неантропо)
$prompts = @(
    # 1: F1 Водные — АНТРОПО (humanoid fish-man)
    "dramatic cinematic concept art portrait of an aquatic alien humanoid, FRONT VIEW, humanoid face with smooth turquoise skin, gill slits on neck, fin-like ears, wide luminous dark eyes, wet glossy skin, chiaroscuro lighting, painterly brushwork, $styleTail",
    # 2: F1 Водные — НЕАНТРОПО (панцирный краб-голова)
    "dramatic cinematic concept art portrait of an alien aquatic crustacean creature, FRONT VIEW, armored crab-like head with turquoise chitin plates, stalk eyes, claw antennae, gill plates, wet sheen, chiaroscuro lighting, painterly brushwork, $styleTail",
    # 3: F4 Серные — АНТРОПО (humanoid demon)
    "dramatic cinematic concept art portrait of an alien sulfur humanoid demon, FRONT VIEW, humanoid face with dark basalt skin, glowing amber slit eyes, curved horns, amber cracks in skin, chiaroscuro lighting, painterly brushwork, $styleTail",
    # 4: F4 Серные — НЕАНТРОПО (каменный монстр-башка)
    "dramatic cinematic concept art portrait of an alien sulfur beast, FRONT VIEW, massive basalt rock monster head, glowing amber cracks, curved horns, heavy jaw, no human features, chiaroscuro lighting, painterly brushwork, $styleTail",
    # 5: F6 Кремниевые — АНТРОПО (humanoid с кристальными гранями)
    "dramatic cinematic concept art portrait of an alien silicon humanoid, FRONT VIEW, humanoid face with faceted crystalline skin, translucent quartz cheeks, glowing crystal eyes, sharp angular features, chiaroscuro lighting, painterly brushwork, $styleTail",
    # 6: F6 Кремниевые — НЕАНТРОПО (абстрактная друза)
    "dramatic cinematic concept art portrait of an alien silicon crystal formation, FRONT VIEW, abstract faceted crystal druse, no face, translucent quartz shards, glowing inner core, specular highlights, chiaroscuro lighting, painterly brushwork, $styleTail",
    # 7: F9 Экзотика — АНТРОПО (humanoid энергетический)
    "dramatic cinematic concept art portrait of an alien energy humanoid, FRONT VIEW, humanoid face of glowing violet energy, soft magnetic arcs around head, radiant pulsing core, ethereal, chiaroscuro lighting, painterly brushwork, $styleTail",
    # 8: F9 Экзотика — НЕАНТРОПО (чистая энергия/дуги)
    "dramatic cinematic concept art portrait of an alien energy being, FRONT VIEW, abstract glowing violet energy sphere with magnetic field arcs, no face, bright pulsing core, radiant, chiaroscuro lighting, painterly brushwork, $styleTail"
)

for ($i = 0; $i -lt 8; $i++) {
    $seed = $StartSeed + $i
    $num = $i + 1
    Write-Host "=== $num/8 [vfork] seed=$seed ==="
    Add-Content $log "--- $num/8 seed=$seed"
    $fin = "$outDir\v${num}.png"
    $r = Send-ComfyPrompt -Prompt $prompts[$i] -NegativePrompt $negArt -Width 1024 -Height 1024 -Steps 40 -Cfg 7 -Seed $seed -Model "juggernaut-xl-v9.safetensors" -OutputPath $fin
    if (-not $r) { Add-Content $log "v$num failed"; continue }
    & $py "C:\Zorion2\tools\process_ship.py" $fin "$procDir\v${num}.png" --no-orient
    Add-Content $log "OK v$num"
}

Add-Content $log "=== races_p6_vfork done $(Get-Date -Format s) ==="
Write-Host "=== Done: 8 avatars (pack 6 vfork: 4 families x anthropo/non-anthropo) ==="