package config

import (
	"encoding/json"
	"fmt"
	"os"
)

// LoadStudio читает и валидирует config/art/studio.json.
// Дефолты — по спеке 67a.1 §4.4 (применяются, если поле не задано).
func LoadStudio(path string) (*StudioConfig, error) {
	cfg := &StudioConfig{}
	if err := readJSON(path, cfg); err != nil {
		return nil, err
	}
	if cfg.Port == 0 {
		cfg.Port = 8798
	}
	if cfg.ComfyURL == "" {
		cfg.ComfyURL = "http://127.0.0.1:8188"
	}
	if cfg.ComfyInput == "" {
		cfg.ComfyInput = `C:\ComfyUI\input`
	}
	if cfg.PythonCmd == "" {
		cfg.PythonCmd = `C:\ComfyUI\venv\Scripts\python.exe`
	}
	if cfg.Checkpoint == "" {
		cfg.Checkpoint = "juggernaut-xl-v9.safetensors"
	}
	if cfg.Steps == 0 {
		cfg.Steps = 28
	}
	if cfg.Cfg == 0 {
		cfg.Cfg = 7.0
	}
	if cfg.CfgImg == 0 {
		cfg.CfgImg = 7.5
	}
	if cfg.DenoiseRef == 0 {
		cfg.DenoiseRef = 0.35
	}
	if cfg.CNStrength == 0 {
		cfg.CNStrength = 1.2
	}
	if cfg.CNEnd == 0 {
		cfg.CNEnd = 0.7
	}
	if cfg.Workers == 0 {
		cfg.Workers = 2
	}
	if cfg.PoolRoot == "" {
		cfg.PoolRoot = "ai_drafts"
	}
	if cfg.PollIntervalS == 0 {
		cfg.PollIntervalS = 2
	}
	if cfg.HistoryTimeoutS == 0 {
		cfg.HistoryTimeoutS = 360
	}
	if cfg.AutoRefreshMS == 0 {
		cfg.AutoRefreshMS = 3000
	}
	if cfg.MaxCount == 0 {
		cfg.MaxCount = 24
	}
	if cfg.RembgCLI == "" {
		return nil, fmt.Errorf("%s: rembg_cli не задан", path)
	}
	return cfg, nil
}

// LoadForms читает и валидирует config/art/forms.json.
func LoadForms(path string) (*FormsConfig, error) {
	fc := &FormsConfig{}
	if err := readJSON(path, fc); err != nil {
		return nil, err
	}
	if fc.PhraseTemplate == "" {
		return nil, fmt.Errorf("%s: phrase_template пуст", path)
	}
	if len(fc.Shapes) == 0 {
		return nil, fmt.Errorf("%s: shapes пуст", path)
	}
	if len(fc.Struct) == 0 || len(fc.Character) == 0 || len(fc.Parts) == 0 {
		return nil, fmt.Errorf("%s: struct/character/parts неполны", path)
	}
	if len(fc.AnthroForms) == 0 {
		return nil, fmt.Errorf("%s: anthro_forms пуст", path)
	}
	if len(fc.CategoryKeys) == 0 {
		return nil, fmt.Errorf("%s: category_keys пуст", path)
	}
	return fc, nil
}

// LoadFamilies читает и валидирует config/art/families.json.
// Инвариант 67a.1 §11.6: у каждой расы forms 2–4, materials 2–3, glows 2–3;
// у семейства extra/anchor/scene/neg непустые.
func LoadFamilies(path string) (FamiliesConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	delete(raw, "comment") // служебное поле файла
	fam := FamiliesConfig{}
	for k, v := range raw {
		var f Family
		if err := json.Unmarshal(v, &f); err != nil {
			return nil, fmt.Errorf("%s: %s: %w", path, k, err)
		}
		fam[k] = f
	}
	if len(fam) == 0 {
		return nil, fmt.Errorf("%s: пусто", path)
	}
	for fid, f := range fam {
		if f.Name == "" {
			return nil, fmt.Errorf("%s: %s: name пуст", path, fid)
		}
		if len(f.Races) == 0 {
			return nil, fmt.Errorf("%s: %s: races пуст", path, fid)
		}
		for _, rc := range f.Races {
			if rc.ID == "" || rc.Name == "" {
				return nil, fmt.Errorf("%s: %s: раса с пустым id/name", path, fid)
			}
			if len(rc.Forms) < 2 || len(rc.Forms) > 4 {
				return nil, fmt.Errorf("%s: %s/%s: forms %d (нужно 2–4)", path, fid, rc.ID, len(rc.Forms))
			}
			if len(rc.Materials) < 2 || len(rc.Materials) > 3 {
				return nil, fmt.Errorf("%s: %s/%s: materials %d (нужно 2–3)", path, fid, rc.ID, len(rc.Materials))
			}
			if len(rc.Glows) < 2 || len(rc.Glows) > 3 {
				return nil, fmt.Errorf("%s: %s/%s: glows %d (нужно 2–3)", path, fid, rc.ID, len(rc.Glows))
			}
		}
		if len(f.Extra) == 0 || len(f.Anchor) == 0 || f.Scene == "" || f.Neg == "" {
			return nil, fmt.Errorf("%s: %s: extra/anchor/scene/neg неполны", path, fid)
		}
	}
	return fam, nil
}

// LoadHumans читает и валидирует config/art/humans.json.
func LoadHumans(path string) (*HumansConfig, error) {
	hc := &HumansConfig{}
	if err := readJSON(path, hc); err != nil {
		return nil, err
	}
	for _, k := range []string{"GENDER", "SKIN", "AGE", "HAIR", "BEARD", "CLOTHES", "ARMOR", "HELMET", "EXPR", "FACIAL", "SCENE"} {
		if len(hc.Axes[k]) == 0 {
			return nil, fmt.Errorf("%s: ось %s пуста", path, k)
		}
	}
	if hc.Neg == "" {
		return nil, fmt.Errorf("%s: neg пуст", path)
	}
	if hc.PromptTemplate == "" {
		return nil, fmt.Errorf("%s: prompt_template пуст", path)
	}
	if hc.Params.MaxCount == 0 {
		hc.Params.MaxCount = 24
	}
	return hc, nil
}

func readJSON(path string, v interface{}) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(data, v); err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	return nil
}