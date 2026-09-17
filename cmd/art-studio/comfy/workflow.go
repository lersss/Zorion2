package comfy

// Сборка workflow-JSON (перенос submit/submit_ref из race_gen.py, спека 67a.1 §5.1).

// Txt2ImgWorkflow — txt2img (кандидаты эталона, пул):
// CheckpointLoaderSimple → 2×CLIPTextEncode → EmptyLatentImage size×size →
// KSampler (dpmpp_2m/karras, denoise 1.0) → VAEDecode → SaveImage.
func Txt2ImgWorkflow(checkpoint, prompt, neg string, seed, steps int, cfg float64, size int, prefix string) map[string]interface{} {
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

// Img2ImgWorkflow — img2img вариация от эталона (refName — файл в ComfyUI/input):
// LoadImage → SplitImageWithAlpha (альфа-маска) → ThresholdMask(0.5) → MaskToImage
// (белое на чёрном, БЕЗ внутренних текстур) → Canny → ControlNetApplyAdvanced
// (форму держит силуэт эталона; cnStrength — сила формы, cnEnd — до какого шага
// ControlNet активен; арт-ТЗ 67a) + VAEEncode от эталона → KSampler
// (cfg 7.5, denoise = ползунок, positive/negative — CN-обработанные) →
// VAEDecode → SaveImage.
// LoadImage срезает альфу (RGB) — ImageToMask(alpha) падал IndexError; маску
// берём из SplitImageWithAlpha (выход [IMAGE, MASK]).
// size=512: эталон масштабируется ImageScale (lanczos, crop center) до 512 —
// и маска, и VAEEncode идут от масштабированного (латент 512, в ~4 раза быстрее;
// решение создателя 2026-09-17). size=1024: без масштабирования (как раньше).
func Img2ImgWorkflow(checkpoint, prompt, neg, refName string, seed, steps int, cfg, denoise, cnStrength, cnEnd float64, size int, prefix string) map[string]interface{} {
	wf := map[string]interface{}{
		"1":  map[string]interface{}{"class_type": "CheckpointLoaderSimple", "inputs": map[string]interface{}{"ckpt_name": checkpoint}},
		"2":  map[string]interface{}{"class_type": "CLIPTextEncode", "inputs": map[string]interface{}{"text": prompt, "clip": []interface{}{"1", 1}}},
		"3":  map[string]interface{}{"class_type": "CLIPTextEncode", "inputs": map[string]interface{}{"text": neg, "clip": []interface{}{"1", 1}}},
		"8":  map[string]interface{}{"class_type": "LoadImage", "inputs": map[string]interface{}{"image": refName}},
		"9":  map[string]interface{}{"class_type": "SplitImageWithAlpha", "inputs": map[string]interface{}{"image": []interface{}{"8", 0}}},
		"10": map[string]interface{}{"class_type": "ThresholdMask", "inputs": map[string]interface{}{"mask": []interface{}{"9", 1}, "value": 0.5}},
		"11": map[string]interface{}{"class_type": "MaskToImage", "inputs": map[string]interface{}{"mask": []interface{}{"10", 0}}},
		"12": map[string]interface{}{"class_type": "Canny", "inputs": map[string]interface{}{"image": []interface{}{"11", 0}, "low_threshold": 0.2, "high_threshold": 0.5}},
		"13": map[string]interface{}{"class_type": "ControlNetLoader", "inputs": map[string]interface{}{"control_net_name": "controlnet-canny-sdxl-1.0.safetensors"}},
		"14": map[string]interface{}{"class_type": "ControlNetApplyAdvanced", "inputs": map[string]interface{}{
			"positive": []interface{}{"2", 0}, "negative": []interface{}{"3", 0}, "control_net": []interface{}{"13", 0}, "image": []interface{}{"12", 0},
			"strength": cnStrength, "start_percent": 0.0, "end_percent": cnEnd}},
		"4":  map[string]interface{}{"class_type": "VAEEncode", "inputs": map[string]interface{}{"pixels": []interface{}{"8", 0}, "vae": []interface{}{"1", 2}}},
		"5":  map[string]interface{}{"class_type": "KSampler", "inputs": map[string]interface{}{
			"seed": seed, "steps": steps, "cfg": cfg, "sampler_name": "dpmpp_2m", "scheduler": "karras", "denoise": denoise,
			"model": []interface{}{"1", 0}, "positive": []interface{}{"14", 0}, "negative": []interface{}{"14", 1}, "latent_image": []interface{}{"4", 0}}},
		"6":  map[string]interface{}{"class_type": "VAEDecode", "inputs": map[string]interface{}{"samples": []interface{}{"5", 0}, "vae": []interface{}{"1", 2}}},
		"7":  map[string]interface{}{"class_type": "SaveImage", "inputs": map[string]interface{}{"images": []interface{}{"6", 0}, "filename_prefix": prefix}},
	}
	if size == 512 {
		wf["15"] = map[string]interface{}{"class_type": "ImageScale", "inputs": map[string]interface{}{
			"image": []interface{}{"8", 0}, "upscale_method": "lanczos", "width": 512, "height": 512, "crop": "center"}}
		wf["9"] = map[string]interface{}{"class_type": "SplitImageWithAlpha", "inputs": map[string]interface{}{"image": []interface{}{"15", 0}}}
		wf["4"] = map[string]interface{}{"class_type": "VAEEncode", "inputs": map[string]interface{}{"pixels": []interface{}{"15", 0}, "vae": []interface{}{"1", 2}}}
	}
	return wf
}