// Package goodsstudio — домен каталога товаров/ресурсов игрового сервера
// (спека переноса-студии-товаров-iterA §3): чистые пакеты студии
// (model/graph/validate, перенесены из cmd/goods-studio) + сидер каталога.
// Каталог живёт в БД (categories/goods/goods_slots); Go-каталог
// internal/resource — только первичное наполнение (С1).
package goodsstudio

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"zorion/internal/goodsstudio/graph"
	"zorion/internal/goodsstudio/model"
	"zorion/internal/resource"
)

// SeedMarkerKey — ключ маркера сидера в generation_config (спека iterA §4.4):
// payload {"applied_at": ..., "resources": 131, "categories": 19}.
const SeedMarkerKey = "goods_catalog_seed"

// Seed — сидер каталога (спека iterA §5): при первом старте (маркер
// goods_catalog_seed в generation_config) в одной транзакции сеет
// 6 ресурсных категорий (is_system, code из resource.AllCategories),
// 13 товарных категорий (model.DefaultCategories) и 131 ресурс
// (LayerCatalog 20 + RealCatalog 111, source=palette, props JSONB).
// Повторные старты — пропуск (маркер): удалённый ресурс
// не возвращается, правки студии Go-каталогом не перезаписываются (С1).
// Ошибка — возвращается; вызывающий (cmd/server/main.go) делает log.Fatal.
func Seed(db *sql.DB) error {
	var exists bool
	if err := db.QueryRow(
		`SELECT EXISTS(SELECT 1 FROM generation_config WHERE key = $1)`, SeedMarkerKey,
	).Scan(&exists); err != nil {
		return fmt.Errorf("seed: маркер: %w", err)
	}
	if exists {
		return nil
	}

	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("seed: begin: %w", err)
	}
	defer tx.Rollback()

	// 6 ресурсных категорий (системные, code — игровой код категории).
	catByCode := make(map[string]int64, len(resource.AllCategories))
	for _, code := range resource.AllCategories {
		meta := resource.GetCategory(code)
		var id int64
		if err := tx.QueryRow(
			`INSERT INTO categories (name, name_norm, kind, code, is_system) VALUES ($1, $2, 'resource', $3, true) RETURNING id`,
			meta.Name, graph.NormalizeName(meta.Name), code,
		).Scan(&id); err != nil {
			return fmt.Errorf("seed: категория ресурса %s: %w", code, err)
		}
		catByCode[code] = id
	}

	// 13 товарных категорий (стартовое удобство, решение гейта №3).
	for _, name := range model.DefaultCategories {
		if _, err := tx.Exec(
			`INSERT INTO categories (name, name_norm, kind) VALUES ($1, $2, 'good')`,
			name, graph.NormalizeName(name),
		); err != nil {
			return fmt.Errorf("seed: товарная категория %s: %w", name, err)
		}
	}

	// 131 ресурс: слой 20 + витрина 111. Слотов у ресурсов нет (инвариант 2).
	resources := 0
	for _, r := range resource.LayerCatalog() {
		props, err := layerProps(r)
		if err != nil {
			return fmt.Errorf("seed: props %s: %w", r.Name, err)
		}
		if err := insertResource(tx, r.Name, catByCode[r.Category], props); err != nil {
			return err
		}
		resources++
	}
	for _, r := range resource.RealCatalog() {
		props, err := realProps(r)
		if err != nil {
			return fmt.Errorf("seed: props %s: %w", r.Name, err)
		}
		if err := insertResource(tx, r.Name, catByCode[r.Category], props); err != nil {
			return err
		}
		resources++
	}

	// Маркер — в той же транзакции: сид атомарен.
	marker, err := json.Marshal(map[string]interface{}{
		"applied_at": time.Now().UTC().Format(time.RFC3339),
		"resources":  resources,
		"categories": len(resource.AllCategories) + len(model.DefaultCategories),
	})
	if err != nil {
		return fmt.Errorf("seed: маркер: %w", err)
	}
	if _, err := tx.Exec(
		`INSERT INTO generation_config (key, payload) VALUES ($1, $2)`, SeedMarkerKey, string(marker),
	); err != nil {
		return fmt.Errorf("seed: маркер: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("seed: commit: %w", err)
	}
	return nil
}

// insertResource — INSERT ресурса (source=palette, без слотов).
func insertResource(tx *sql.Tx, name string, categoryID int64, props []byte) error {
	if _, err := tx.Exec(
		`INSERT INTO goods (name, name_norm, category_id, kind, source, props)
		 VALUES ($1, $2, $3, 'resource', 'palette', $4)`,
		name, graph.NormalizeName(name), categoryID, props,
	); err != nil {
		return fmt.Errorf("seed: ресурс %s: %w", name, err)
	}
	return nil
}

// layerProps — props ресурса слоя (спека iterA §4.2): 10 осей (ключи —
// русские имена осей из layer.go), t_melt_k/t_boil_k (K), closes, bridge,
// supercritical (только layer).
func layerProps(r *resource.Resource) ([]byte, error) {
	p := map[string]interface{}{
		resource.AxisHardness:         r.Hardness,
		resource.AxisElasticity:       r.Elasticity,
		resource.AxisConductivity:     r.Conductivity,
		resource.AxisDensity:          r.Density,
		resource.AxisEnergyDensity:    r.EnergyDensity,
		resource.AxisBiocompatibility: r.Biocompatibility,
		resource.AxisRadioactivity:    r.Radioactivity,
		resource.AxisToxicity:         r.Toxicity,
		resource.AxisFlammability:     r.Flammability,
		resource.AxisChemicalActivity: r.ChemicalActivity,
		"t_melt_k":                    r.TMelt,
		"t_boil_k":                    r.TBoil,
		"closes":                      r.Closes,
		"bridge":                      r.Bridge,
		"supercritical":               r.Supercritical,
	}
	return json.Marshal(p)
}

// realProps — props реального вещества (спека iterA §4.2): 10 осей,
// t_melt_k/t_boil_k (K), family (только real).
func realProps(r *resource.RealResource) ([]byte, error) {
	p := map[string]interface{}{
		resource.AxisHardness:         r.Hardness,
		resource.AxisElasticity:       r.Elasticity,
		resource.AxisConductivity:     r.Conductivity,
		resource.AxisDensity:          r.Density,
		resource.AxisEnergyDensity:    r.EnergyDensity,
		resource.AxisBiocompatibility: r.Biocompatibility,
		resource.AxisRadioactivity:    r.Radioactivity,
		resource.AxisToxicity:         r.Toxicity,
		resource.AxisFlammability:     r.Flammability,
		resource.AxisChemicalActivity: r.ChemicalActivity,
		"t_melt_k":                    r.TMelt,
		"t_boil_k":                    r.TBoil,
		"family":                      r.Family,
	}
	return json.Marshal(p)
}