// internal/generator/planet/biome_catalog_seed.go
//
// Встроенный сид справочника биомов (99.2.28 §15.1): полный каталог
// 57 биомов + 17 типов недр + правила типов планет (приложение
// 99.2.28-appendix-biome-catalog.md §1–§3). Источник определений 41 нового
// биома — материал @scientist/@writer. Константы composition_forms.go —
// сид имён для обратной совместимости старых миров.
//
// Файл biome_catalog_seed.json — заводской каталог; config/biome_catalog.json
// — рабочая копия (грузится при старте, правится админкой). «Сбросить к
// заводским» перезагружает этот сид.
package planet

import (
	_ "embed"
	"encoding/json"
	"log"
)

//go:embed biome_catalog_seed.json
var biomeCatalogSeedJSON []byte

// defaultBiomeCatalog — встроенный сид справочника. Парсится один раз при
// инициализации пакета; битый сид — ошибка разработки (log.Fatal).
func defaultBiomeCatalog() *BiomeCatalog {
	var cat BiomeCatalog
	if err := json.Unmarshal(biomeCatalogSeedJSON, &cat); err != nil {
		log.Fatalf("❌ Встроенный сид справочника биомов: битый JSON: %v", err)
	}
	if err := cat.Validate(); err != nil {
		log.Fatalf("❌ Встроенный сид справочника биомов: валидация: %v", err)
	}
	return &cat
}