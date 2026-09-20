package comfy

import "testing"

// TestShipStage1Workflow — этап 1: Canny 0.2/0.5, ControlNet strength 1.5,
// end 1.0, denoise 0.85, cfg 7.0 (спека §3.2, art_ships §2.2). size=1024 —
// полный, без масштабирования.
func TestShipStage1Workflow(t *testing.T) {
	wf := ShipStage1Workflow("ckpt", "p", "n", "ship_sil_humans.png", 42, 40, 7.0, 1.5, 1024, "ship_pool")
	canny := wf["12"].(map[string]interface{})["inputs"].(map[string]interface{})
	if canny["low_threshold"] != 0.2 || canny["high_threshold"] != 0.5 {
		t.Errorf("canny = %v/%v, want 0.2/0.5", canny["low_threshold"], canny["high_threshold"])
	}
	cn := wf["14"].(map[string]interface{})["inputs"].(map[string]interface{})
	if cn["strength"] != 1.5 || cn["end_percent"] != 1.0 {
		t.Errorf("controlnet strength/end = %v/%v, want 1.5/1.0", cn["strength"], cn["end_percent"])
	}
	ks := wf["5"].(map[string]interface{})["inputs"].(map[string]interface{})
	if ks["denoise"] != 0.85 || ks["cfg"] != 7.0 || ks["steps"] != 40 {
		t.Errorf("ksampler denoise/cfg/steps = %v/%v/%v, want 0.85/7.0/40", ks["denoise"], ks["cfg"], ks["steps"])
	}
	// LoadImage — силуэт
	li := wf["8"].(map[string]interface{})["inputs"].(map[string]interface{})
	if li["image"] != "ship_sil_humans.png" {
		t.Errorf("loadimage = %v", li["image"])
	}
}

// TestShipStage2Workflow — этап 2: img2img без ControlNet, denoise 0.45–0.55,
// cfg 7.5 (спека §3.3, art_ships §2.3). size=1024 — полный, без масштабирования.
func TestShipStage2Workflow(t *testing.T) {
	wf := ShipStage2Workflow("ckpt", "p", "n", "raw1.png", 42, 40, 7.5, 0.5, 1024, "ship_pool")
	if _, ok := wf["12"]; ok {
		t.Errorf("этап 2 не должен содержать Canny/ControlNet")
	}
	ks := wf["5"].(map[string]interface{})["inputs"].(map[string]interface{})
	if ks["denoise"] != 0.5 || ks["cfg"] != 7.5 {
		t.Errorf("ksampler denoise/cfg = %v/%v, want 0.5/7.5", ks["denoise"], ks["cfg"])
	}
	// positive/negative — напрямую от CLIPTextEncode (без ControlNet)
	if ks["positive"].([]interface{})[0] != "2" {
		t.Errorf("positive не от узла 2")
	}
}

// TestShipWorkflowSize — эскиз (size=512): ImageScale 512 (lanczos, crop center)
// в обоих этапах, Canny/VAEEncode от масштабированного (латент 512, ~4x
// быстрее); полный (size=1024) — без ImageScale (regression: эскиз гнал
// img2img на 1024 — не ускорял).
func TestShipWorkflowSize(t *testing.T) {
	// эскиз: этап 1
	wf1 := ShipStage1Workflow("ckpt", "p", "n", "ship_sil_humans.png", 42, 40, 7.0, 1.5, 512, "ship_pool")
	scale1, ok := wf1["15"].(map[string]interface{})
	if !ok {
		t.Fatalf("этап 1 (эскиз): нет узла 15 ImageScale")
	}
	in1 := scale1["inputs"].(map[string]interface{})
	if in1["width"] != 512 || in1["height"] != 512 || in1["upscale_method"] != "lanczos" || in1["crop"] != "center" {
		t.Errorf("ImageScale этапа 1 = %v, want 512×512 lanczos center", in1)
	}
	// Canny и VAEEncode — от масштабированного (узел 15)
	canny := wf1["12"].(map[string]interface{})["inputs"].(map[string]interface{})
	if canny["image"].([]interface{})[0] != "15" {
		t.Errorf("Canny этапа 1 не от масштабированного: %v", canny["image"])
	}
	vae := wf1["4"].(map[string]interface{})["inputs"].(map[string]interface{})
	if vae["pixels"].([]interface{})[0] != "15" {
		t.Errorf("VAEEncode этапа 1 не от масштабированного: %v", vae["pixels"])
	}
	// эскиз: этап 2
	wf2 := ShipStage2Workflow("ckpt", "p", "n", "raw1.png", 42, 40, 7.5, 0.5, 512, "ship_pool")
	scale2, ok := wf2["15"].(map[string]interface{})
	if !ok {
		t.Fatalf("этап 2 (эскиз): нет узла 15 ImageScale")
	}
	in2 := scale2["inputs"].(map[string]interface{})
	if in2["width"] != 512 || in2["height"] != 512 {
		t.Errorf("ImageScale этапа 2 = %v, want 512×512", in2)
	}
	vae2 := wf2["4"].(map[string]interface{})["inputs"].(map[string]interface{})
	if vae2["pixels"].([]interface{})[0] != "15" {
		t.Errorf("VAEEncode этапа 2 не от масштабированного: %v", vae2["pixels"])
	}
	// полный: без ImageScale (regression)
	wf1f := ShipStage1Workflow("ckpt", "p", "n", "ship_sil_humans.png", 42, 40, 7.0, 1.5, 1024, "ship_pool")
	if _, ok := wf1f["15"]; ok {
		t.Errorf("этап 1 (полный): не должно быть ImageScale")
	}
	wf2f := ShipStage2Workflow("ckpt", "p", "n", "raw1.png", 42, 40, 7.5, 0.5, 1024, "ship_pool")
	if _, ok := wf2f["15"]; ok {
		t.Errorf("этап 2 (полный): не должно быть ImageScale")
	}
}