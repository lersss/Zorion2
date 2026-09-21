package comfy

// Воркфлоу генерации кораблей рас.
//
// Актуальный рецепт (2026-09-21, проверен спайками): чистый txt2img
// (ShipTxt2ImgWorkflow) + вырез (tools/ship_sprite_cut.py) + этап детализации
// Hi-Res (ShipHiResWorkflow) на кандидатах, прошедших автопроверку кадра.
// Ниже остались ShipStage1/2Workflow прежнего конвейера «силуэт → ControlNet»
// — они выведены из нового джоба и станут мёртвыми (уборка — отдельным
// решением, спека 2026-09-21 §12).

// ShipTxt2ImgWorkflow — новый этап 1 (рецепт 2026-09-21): чистый txt2img без
// ControlNet и без силуэта. CheckpointLoaderSimple → 2×CLIPTextEncode →
// EmptyLatentImage size×size → KSampler (denoise 1.0, dpmpp_2m/karras) →
// VAEDecode → SaveImage. size=1024 — полный кадр, size=512 — эскиз.
func ShipTxt2ImgWorkflow(checkpoint, prompt, neg string, seed, steps int, cfg float64, size int, prefix string) map[string]interface{} {
	return map[string]interface{}{
		"1": map[string]interface{}{"class_type": "CheckpointLoaderSimple", "inputs": map[string]interface{}{"ckpt_name": checkpoint}},
		"2": map[string]interface{}{"class_type": "CLIPTextEncode", "inputs": map[string]interface{}{"text": prompt, "clip": []interface{}{"1", 1}}},
		"3": map[string]interface{}{"class_type": "CLIPTextEncode", "inputs": map[string]interface{}{"text": neg, "clip": []interface{}{"1", 1}}},
		"4": map[string]interface{}{"class_type": "EmptyLatentImage", "inputs": map[string]interface{}{"width": size, "height": size, "batch_size": 1}},
		"5": map[string]interface{}{"class_type": "KSampler", "inputs": map[string]interface{}{
			"seed": seed, "steps": steps, "cfg": cfg, "sampler_name": "dpmpp_2m", "scheduler": "karras", "denoise": 1.0,
			"model": []interface{}{"1", 0}, "positive": []interface{}{"2", 0}, "negative": []interface{}{"3", 0}, "latent_image": []interface{}{"4", 0}}},
		"6": map[string]interface{}{"class_type": "VAEDecode", "inputs": map[string]interface{}{"samples": []interface{}{"5", 0}, "vae": []interface{}{"1", 2}}},
		"7": map[string]interface{}{"class_type": "SaveImage", "inputs": map[string]interface{}{"images": []interface{}{"6", 0}, "filename_prefix": prefix}},
	}
}

// ShipHiResWorkflow — этап детализации (рецепт 2026-09-21): LoadImage →
// UpscaleModelLoader (4x-UltraSharp) → ImageUpscaleWithModel → ImageScale
// (lanczos, scale×scale) → VAEEncode → KSampler (denoise ~0.40, dpmpp_2m/
// karras) → VAEDecode → SaveImage. imgName — файл в ComfyUI/input.
func ShipHiResWorkflow(checkpoint, prompt, neg, imgName string, seed, steps int, cfg, denoise float64, upscaler string, scale int, prefix string) map[string]interface{} {
	return map[string]interface{}{
		"1":  map[string]interface{}{"class_type": "CheckpointLoaderSimple", "inputs": map[string]interface{}{"ckpt_name": checkpoint}},
		"2":  map[string]interface{}{"class_type": "CLIPTextEncode", "inputs": map[string]interface{}{"text": prompt, "clip": []interface{}{"1", 1}}},
		"3":  map[string]interface{}{"class_type": "CLIPTextEncode", "inputs": map[string]interface{}{"text": neg, "clip": []interface{}{"1", 1}}},
		"8":  map[string]interface{}{"class_type": "LoadImage", "inputs": map[string]interface{}{"image": imgName}},
		"12": map[string]interface{}{"class_type": "UpscaleModelLoader", "inputs": map[string]interface{}{"model_name": upscaler}},
		"13": map[string]interface{}{"class_type": "ImageUpscaleWithModel", "inputs": map[string]interface{}{"upscale_model": []interface{}{"12", 0}, "image": []interface{}{"8", 0}}},
		"14": map[string]interface{}{"class_type": "ImageScale", "inputs": map[string]interface{}{
			"image": []interface{}{"13", 0}, "width": scale, "height": scale, "upscale_method": "lanczos", "crop": "disabled"}},
		"4": map[string]interface{}{"class_type": "VAEEncode", "inputs": map[string]interface{}{"pixels": []interface{}{"14", 0}, "vae": []interface{}{"1", 2}}},
		"5": map[string]interface{}{"class_type": "KSampler", "inputs": map[string]interface{}{
			"seed": seed, "steps": steps, "cfg": cfg, "sampler_name": "dpmpp_2m", "scheduler": "karras", "denoise": denoise,
			"model": []interface{}{"1", 0}, "positive": []interface{}{"2", 0}, "negative": []interface{}{"3", 0}, "latent_image": []interface{}{"4", 0}}},
		"6": map[string]interface{}{"class_type": "VAEDecode", "inputs": map[string]interface{}{"samples": []interface{}{"5", 0}, "vae": []interface{}{"1", 2}}},
		"7": map[string]interface{}{"class_type": "SaveImage", "inputs": map[string]interface{}{"images": []interface{}{"6", 0}, "filename_prefix": prefix}},
	}
}

// ShipStage1Workflow — этап 1 (форма, ControlNet Canny + img2img):
// LoadImage(силуэт) → Canny (low 0.2 / high 0.5) → ControlNetApplyAdvanced
// (strength 1.5, start 0.0, end 1.0) + VAEEncode(силуэт) → KSampler
// (denoise 0.85, steps 40, cfg 7.0, dpmpp_2m/karras) → VAEDecode → SaveImage.
// silName — файл силуэта в ComfyUI/input (ship_sil_<slug>.png).
// size=512 (эскиз): силуэт масштабируется ImageScale (lanczos, crop center)
// до 512 — Canny/ControlNet и VAEEncode от масштабированного (латент 512,
// в ~4 раза быстрее; как Img2ImgWorkflow). size=1024: без масштабирования.
func ShipStage1Workflow(checkpoint, prompt, neg, silName string, seed, steps int, cfg, cnStrength float64, size int, prefix string) map[string]interface{} {
	wf := map[string]interface{}{
		"1":  map[string]interface{}{"class_type": "CheckpointLoaderSimple", "inputs": map[string]interface{}{"ckpt_name": checkpoint}},
		"2":  map[string]interface{}{"class_type": "CLIPTextEncode", "inputs": map[string]interface{}{"text": prompt, "clip": []interface{}{"1", 1}}},
		"3":  map[string]interface{}{"class_type": "CLIPTextEncode", "inputs": map[string]interface{}{"text": neg, "clip": []interface{}{"1", 1}}},
		"8":  map[string]interface{}{"class_type": "LoadImage", "inputs": map[string]interface{}{"image": silName}},
		"12": map[string]interface{}{"class_type": "Canny", "inputs": map[string]interface{}{"image": []interface{}{"8", 0}, "low_threshold": 0.2, "high_threshold": 0.5}},
		"13": map[string]interface{}{"class_type": "ControlNetLoader", "inputs": map[string]interface{}{"control_net_name": "controlnet-canny-sdxl-1.0.safetensors"}},
		"14": map[string]interface{}{"class_type": "ControlNetApplyAdvanced", "inputs": map[string]interface{}{
			"positive": []interface{}{"2", 0}, "negative": []interface{}{"3", 0}, "control_net": []interface{}{"13", 0}, "image": []interface{}{"12", 0},
			"strength": cnStrength, "start_percent": 0.0, "end_percent": 1.0}},
		"4":  map[string]interface{}{"class_type": "VAEEncode", "inputs": map[string]interface{}{"pixels": []interface{}{"8", 0}, "vae": []interface{}{"1", 2}}},
		"5":  map[string]interface{}{"class_type": "KSampler", "inputs": map[string]interface{}{
			"seed": seed, "steps": steps, "cfg": cfg, "sampler_name": "dpmpp_2m", "scheduler": "karras", "denoise": 0.85,
			"model": []interface{}{"1", 0}, "positive": []interface{}{"14", 0}, "negative": []interface{}{"14", 1}, "latent_image": []interface{}{"4", 0}}},
		"6":  map[string]interface{}{"class_type": "VAEDecode", "inputs": map[string]interface{}{"samples": []interface{}{"5", 0}, "vae": []interface{}{"1", 2}}},
		"7":  map[string]interface{}{"class_type": "SaveImage", "inputs": map[string]interface{}{"images": []interface{}{"6", 0}, "filename_prefix": prefix}},
	}
	if size == 512 {
		wf["15"] = map[string]interface{}{"class_type": "ImageScale", "inputs": map[string]interface{}{
			"image": []interface{}{"8", 0}, "upscale_method": "lanczos", "width": 512, "height": 512, "crop": "center"}}
		wf["12"] = map[string]interface{}{"class_type": "Canny", "inputs": map[string]interface{}{"image": []interface{}{"15", 0}, "low_threshold": 0.2, "high_threshold": 0.5}}
		wf["4"] = map[string]interface{}{"class_type": "VAEEncode", "inputs": map[string]interface{}{"pixels": []interface{}{"15", 0}, "vae": []interface{}{"1", 2}}}
	}
	return wf
}

// ShipStage2Workflow — этап 2 (текстура, img2img без ControlNet):
// LoadImage(raw этапа 1) → VAEEncode → KSampler (denoise 0.45–0.55, steps 40,
// cfg 7.5, dpmpp_2m/karras) → VAEDecode → SaveImage.
// size=512 (эскиз): raw1 масштабируется ImageScale (lanczos, crop center)
// до 512 — VAEEncode от масштабированного (латент 512, в ~4 раза быстрее).
// size=1024: без масштабирования.
func ShipStage2Workflow(checkpoint, prompt, neg, imgName string, seed, steps int, cfg, denoise float64, size int, prefix string) map[string]interface{} {
	wf := map[string]interface{}{
		"1": map[string]interface{}{"class_type": "CheckpointLoaderSimple", "inputs": map[string]interface{}{"ckpt_name": checkpoint}},
		"2": map[string]interface{}{"class_type": "CLIPTextEncode", "inputs": map[string]interface{}{"text": prompt, "clip": []interface{}{"1", 1}}},
		"3": map[string]interface{}{"class_type": "CLIPTextEncode", "inputs": map[string]interface{}{"text": neg, "clip": []interface{}{"1", 1}}},
		"8": map[string]interface{}{"class_type": "LoadImage", "inputs": map[string]interface{}{"image": imgName}},
		"4": map[string]interface{}{"class_type": "VAEEncode", "inputs": map[string]interface{}{"pixels": []interface{}{"8", 0}, "vae": []interface{}{"1", 2}}},
		"5": map[string]interface{}{"class_type": "KSampler", "inputs": map[string]interface{}{
			"seed": seed, "steps": steps, "cfg": cfg, "sampler_name": "dpmpp_2m", "scheduler": "karras", "denoise": denoise,
			"model": []interface{}{"1", 0}, "positive": []interface{}{"2", 0}, "negative": []interface{}{"3", 0}, "latent_image": []interface{}{"4", 0}}},
		"6": map[string]interface{}{"class_type": "VAEDecode", "inputs": map[string]interface{}{"samples": []interface{}{"5", 0}, "vae": []interface{}{"1", 2}}},
		"7": map[string]interface{}{"class_type": "SaveImage", "inputs": map[string]interface{}{"images": []interface{}{"6", 0}, "filename_prefix": prefix}},
	}
	if size == 512 {
		wf["15"] = map[string]interface{}{"class_type": "ImageScale", "inputs": map[string]interface{}{
			"image": []interface{}{"8", 0}, "upscale_method": "lanczos", "width": 512, "height": 512, "crop": "center"}}
		wf["4"] = map[string]interface{}{"class_type": "VAEEncode", "inputs": map[string]interface{}{"pixels": []interface{}{"15", 0}, "vae": []interface{}{"1", 2}}}
	}
	return wf
}