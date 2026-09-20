package config

import (
	"encoding/json"
	"fmt"
	"strings"
)

// ShipDictConfig — словари генератора кораблей (config/art/ship_dict.json,
// спека 2026-09-20-ships-races-generator §4.2): формы (рус. ключ → англ.
// концепт этапа 1 + категория скрипта силуэтов), детали (рус. слово → англ.
// фраза этапа 1), тёплые маркеры (гейт тёплых текстурных тегов этапа 2),
// пул текстурных тегов этапа 2, расовые концепт-слова (slug → «корабль
// в образе …», начало промпта этапа 1).
type ShipDictConfig struct {
	Forms       []ShipForm         `json:"forms"`
	Details     []ShipDetail       `json:"details"`
	WarmMarkers []string           `json:"warm_markers"`
	Textures    []ShipTexture      `json:"textures"`
	Concepts    map[string]string  `json:"concepts"`
}

// ShipForm — форма корабля: русское ключевое слово ТЗ → англ. концепт-слово
// (этап 1) + категория формы для скрипта силуэтов (Python draw_<category>).
type ShipForm struct {
	Key      string `json:"key"`
	Concept  string `json:"concept"`
	Category string `json:"category"`
}

// ShipDetail — деталь корабля: русское слово ТЗ (key) + стем для падежных
// форм (stem, «шлейфами» → «шлейф») → англ. фраза (этап 1).
type ShipDetail struct {
	Key    string `json:"key"`
	Stem   string `json:"stem"`
	Phrase string `json:"phrase"`
}

// ShipTexture — текстурный тег этапа 2: warmth ∈ {warm, neutral, cold};
// blocked_by — токены, любой из которых в blocked расы исключает тег.
type ShipTexture struct {
	Tag       string   `json:"tag"`
	Warmth    string   `json:"warmth"`
	BlockedBy []string `json:"blocked_by"`
}

// shipFormCategories — библиотека форм скрипта силуэтов (спека §3.1, 13 шт).
var shipFormCategories = map[string]bool{
	"wing": true, "capsule": true, "barge": true, "crucible": true,
	"envelope": true, "vessel": true, "flask": true, "drop": true,
	"wedge": true, "disc": true, "ring": true, "sphere": true, "swarm": true,
}

// LoadShipDict читает и валидирует config/art/ship_dict.json.
func LoadShipDict(path string) (*ShipDictConfig, error) {
	dc := &ShipDictConfig{}
	if err := readJSON(path, dc); err != nil {
		return nil, err
	}
	if err := validateShipDict(dc, path); err != nil {
		return nil, err
	}
	return dc, nil
}

// ParseShipDict — валидация ship_dict.json из байтов (общая для LoadShipDict
// и будущих пересборок).
func ParseShipDict(data []byte, path string) (*ShipDictConfig, error) {
	dc := &ShipDictConfig{}
	if err := json.Unmarshal(data, dc); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if err := validateShipDict(dc, path); err != nil {
		return nil, err
	}
	return dc, nil
}

// validateShipDict — общая валидация словаря (спека §4.2): forms — key/concept
// непустые, category ∈ библиотека форм скрипта; details — key/phrase непустые;
// warm_markers — непустые элементы, без дублей; textures — tag непустой,
// warmth ∈ {warm, neutral, cold}, blocked_by — токены непустые (пустой список
// допустим), без дублей tag; concepts — концепт непустой, ≤ 60 символов,
// без \n (отсутствие концепта расы валидно — фолбек на форму).
func validateShipDict(dc *ShipDictConfig, path string) error {
	if len(dc.Forms) == 0 {
		return fmt.Errorf("%s: forms пуст", path)
	}
	for _, f := range dc.Forms {
		if strings.TrimSpace(f.Key) == "" || strings.TrimSpace(f.Concept) == "" {
			return fmt.Errorf("%s: форма с пустым key/concept", path)
		}
		if !shipFormCategories[f.Category] {
			return fmt.Errorf("%s: форма %q: категория %q не в библиотеке скрипта", path, f.Key, f.Category)
		}
	}
	if len(dc.Details) == 0 {
		return fmt.Errorf("%s: details пуст", path)
	}
	for _, d := range dc.Details {
		if strings.TrimSpace(d.Key) == "" || strings.TrimSpace(d.Stem) == "" || strings.TrimSpace(d.Phrase) == "" {
			return fmt.Errorf("%s: деталь с пустым key/stem/phrase", path)
		}
	}
	if len(dc.WarmMarkers) == 0 {
		return fmt.Errorf("%s: warm_markers пуст", path)
	}
	seen := map[string]bool{}
	for _, m := range dc.WarmMarkers {
		if strings.TrimSpace(m) == "" {
			return fmt.Errorf("%s: warm_markers содержит пустой элемент", path)
		}
		if seen[m] {
			return fmt.Errorf("%s: warm_markers дубль %q", path, m)
		}
		seen[m] = true
	}
	if len(dc.Textures) == 0 {
		return fmt.Errorf("%s: textures пуст", path)
	}
	tseen := map[string]bool{}
	for _, t := range dc.Textures {
		if strings.TrimSpace(t.Tag) == "" {
			return fmt.Errorf("%s: тег с пустым tag", path)
		}
		if t.Warmth != "warm" && t.Warmth != "neutral" && t.Warmth != "cold" {
			return fmt.Errorf("%s: тег %q: warmth %q (нужно warm/neutral/cold)", path, t.Tag, t.Warmth)
		}
		for _, tok := range t.BlockedBy {
			if strings.TrimSpace(tok) == "" {
				return fmt.Errorf("%s: тег %q: blocked_by содержит пустой токен", path, t.Tag)
			}
		}
		if tseen[t.Tag] {
			return fmt.Errorf("%s: дубль тега %q", path, t.Tag)
		}
		tseen[t.Tag] = true
	}
	for slug, c := range dc.Concepts {
		if strings.TrimSpace(c) == "" {
			return fmt.Errorf("%s: концепт расы %q пуст", path, slug)
		}
		if strings.Contains(c, "\n") {
			return fmt.Errorf("%s: концепт расы %q содержит перенос строки", path, slug)
		}
		if len(c) > 60 {
			return fmt.Errorf("%s: концепт расы %q длиннее 60 символов", path, slug)
		}
	}
	return nil
}