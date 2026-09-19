package catalog

import (
	"encoding/json"
	"fmt"
	"strings"

	"zorion/internal/goodsstudio/graph"
	"zorion/internal/goodsstudio/model"
	"zorion/internal/goodsstudio/validate"
)

// TopCatalog — формат goods_data/top_catalog.json (спека 99a.2 §3.1).
// Файл только читается, не модифицируется (решение создателя п.5).
type TopCatalog struct {
	SchemaVersion int          `json:"schema_version"`
	Catalog       string       `json:"catalog"`
	GeneratedAt   string       `json:"generated_at"`
	Summary       TopSummary   `json:"summary"`
	Goods         []TopGood    `json:"goods"`
}

// TopSummary — справочная сводка каталога (не импортируется, 99a.2 §3.3).
type TopSummary struct {
	Total       int            `json:"total"`
	PerCategory map[string]int `json:"per_category"`
	Demand      struct {
		Continuous int `json:"continuous"`
		Spike      int `json:"spike"`
	} `json:"demand"`
}

// TopGood — товар-верхушка каталога.
type TopGood struct {
	ID           string           `json:"id"`
	Name         string           `json:"name"`
	Category     string           `json:"category"`
	WhyTop       string           `json:"why_top"` // НЕ импортируется (решение создателя п.3)
	Constituents []TopConstituent `json:"constituents"`
}

// TopConstituent — составляющая товара каталога (только name+type, id нет).
type TopConstituent struct {
	Name string `json:"name"`
	Type string `json:"type"` // "resource" | "good"
}

// ImportReport — отчёт импорта (спека 99a.2 §7): что создано/снесено,
// путь бэкапа, предупреждения (парсер + валидаторы). Для деревьев Т8
// (schema 2, trees.go): Imported = узлов создано (после дедупа базы),
// ComponentsCreated = 0, Trees/BaseDeduplicated заполнены.
type ImportReport struct {
	Imported          int      `json:"imported"`           // schema 1: топ-товаров; schema 2: узлов создано
	ComponentsCreated int      `json:"components_created"` // schema 1: внешних good-компонентов; schema 2: 0
	GoodsRemoved      int      `json:"goods_removed"`      // товаров студии снесено
	CategoriesCreated int      `json:"categories_created"` // категорий создано (по имени не найдено)
	Backup            string   `json:"backup"`             // путь бэкапа state.json.bak (заполняет хендлер)
	Warnings          []string `json:"warnings"`           // предупреждения парсера + валидаторов
	Trees             int      `json:"trees,omitempty"`             // schema 2: деревьев в файле
	NodesCreated      int      `json:"nodes_created,omitempty"`     // schema 2: узлов создано (после дедупа базы)
	BaseDeduplicated  int      `json:"base_deduplicated,omitempty"` // schema 2: узлов общей базы пропущено (id уже импортирован)
}

// ParseTopCatalog читает и валидирует top_catalog.json (спека 99a.2 §3.2).
// Ошибка — перечень всех проблем (400, импорт не идёт).
func ParseTopCatalog(data []byte) (*TopCatalog, error) {
	var tc TopCatalog
	if err := json.Unmarshal(data, &tc); err != nil {
		return nil, fmt.Errorf("невалидный JSON: %v", err)
	}
	var problems []string
	if tc.SchemaVersion != 1 {
		problems = append(problems, fmt.Sprintf("неподдерживаемая версия каталога: %d (ожидается 1)", tc.SchemaVersion))
	}
	if len(tc.Goods) == 0 {
		problems = append(problems, "каталог пуст: goods не содержит товаров")
	}
	seenID := make(map[string]bool, len(tc.Goods))
	seenName := make(map[string]bool, len(tc.Goods))
	for i, g := range tc.Goods {
		id := strings.TrimSpace(g.ID)
		if id == "" {
			problems = append(problems, fmt.Sprintf("товар %d: пустой id", i+1))
		} else if seenID[id] {
			problems = append(problems, fmt.Sprintf("дубликат id: %s", id))
		} else {
			seenID[id] = true
		}
		if strings.TrimSpace(g.Name) == "" {
			problems = append(problems, fmt.Sprintf("товар %s: пустое имя", id))
		}
		if strings.TrimSpace(g.Category) == "" {
			problems = append(problems, fmt.Sprintf("товар %s: пустая категория", id))
		}
		norm := graph.NormalizeName(g.Name)
		if norm != "" {
			if seenName[norm] {
				problems = append(problems, fmt.Sprintf("дубликат имени: %s", g.Name))
			} else {
				seenName[norm] = true
			}
		}
		if len(g.Constituents) == 0 {
			problems = append(problems, fmt.Sprintf("товар %s: пустой состав (constituents)", id))
		}
		for j, c := range g.Constituents {
			if strings.TrimSpace(c.Name) == "" {
				problems = append(problems, fmt.Sprintf("товар %s: составляющая %d: пустое имя", id, j+1))
			}
			if c.Type != "resource" && c.Type != "good" {
				problems = append(problems, fmt.Sprintf("товар %s: составляющая %q: невалидный тип %q (ожидается resource|good)", id, c.Name, c.Type))
			}
		}
	}
	if len(problems) > 0 {
		return nil, fmt.Errorf("каталог невалиден:\n- %s", strings.Join(problems, "\n- "))
	}
	return &tc, nil
}

// ApplyTopCatalog — полная замена товаров студии каталогом (спека 99a.2 §4).
// Ресурсы (kind=resource) сохраняются со всеми полями (статусы/баны —
// память статусов); товары (kind=good) сносятся; категории маппятся по
// нормализованному имени (новые — создаются); внешние good-компоненты
// (ext:<slug>, draft, пустые рецепты) создаются; топ-товары (tg_XXXX,
// approved) — со слотами quantity=1, резолв по имени. Мутирует st.
// Возвращает отчёт (Backup заполняет вызывающий хендлер).
func ApplyTopCatalog(st *model.State, tc *TopCatalog) *ImportReport {
	rep := &ImportReport{}

	// шаг 2: сохранить ресурсы со всеми полями (статусы/баны сохраняются)
	resources := make([]model.Good, 0, len(st.Goods))
	for i := range st.Goods {
		if st.Goods[i].Kind == model.KindResource {
			resources = append(resources, st.Goods[i])
		}
	}
	// шаг 3: снести товары (kind=good) — не переносим их
	rep.GoodsRemoved = len(st.Goods) - len(resources)
	st.Goods = resources

	// шаг 4: категории — существующие как есть; каталог маппится по
	// нормализованному имени; не найденная — создаётся (решение создателя п.4)
	catByNorm := make(map[string]string, len(st.Categories))
	for _, c := range st.Categories {
		catByNorm[graph.NormalizeName(c.Name)] = c.ID
	}
	topCatID := make([]string, len(tc.Goods))
	for i, tg := range tc.Goods {
		norm := graph.NormalizeName(tg.Category)
		id, ok := catByNorm[norm]
		if !ok {
			id = model.NextCategoryID(st.Categories)
			st.Categories = append(st.Categories, model.Category{ID: id, Name: tg.Category})
			catByNorm[norm] = id
			rep.CategoriesCreated++
		}
		topCatID[i] = id
	}

	// шаг 5: внешние good-компоненты — уникальные имена составляющих
	// type=good (в порядке файла); категория = категория первого
	// топ-товара-потребителя (развилка 3); статус draft, рецепт пуст
	extByName := make(map[string]string, 8)
	for i, tg := range tc.Goods {
		for _, c := range tg.Constituents {
			if c.Type != "good" {
				continue
			}
			norm := graph.NormalizeName(c.Name)
			if _, ok := extByName[norm]; ok {
				continue
			}
			id := "ext:" + slug(c.Name)
			extByName[norm] = id
			st.Goods = append(st.Goods, model.Good{
				ID:        id,
				Name:      c.Name,
				Category:  topCatID[i],
				Status:    model.StatusDraft,
				Kind:      model.KindGood,
				Source:    model.SourceImport,
				Recipe:    []model.Slot{},
				CreatedAt: model.NowISO(),
			})
			rep.ComponentsCreated++
		}
	}

	// шаги 6–7: топ-товары — слот на каждую составляющую (quantity=1,
	// allow_resource=false, reason пуст), резолв по имени (99a.2 §4.4)
	for i, tg := range tc.Goods {
		g := model.Good{
			ID:        tg.ID,
			Name:      tg.Name,
			Category:  topCatID[i],
			Status:    model.StatusApproved, // решение создателя п.2 — сразу согласовано
			Kind:      model.KindGood,
			Source:    model.SourceImport,
			Recipe:    make([]model.Slot, 0, len(tg.Constituents)),
			CreatedAt: model.NowISO(),
		}
		for _, c := range tg.Constituents {
			slot := model.Slot{Quantity: 1}
			if comp := graph.FindByName(c.Name, st.Goods); comp != nil {
				slot.GoodID = comp.ID
			} else {
				rep.Warnings = append(rep.Warnings, fmt.Sprintf("не найден компонент по имени: %s (товар %s)", c.Name, tg.Name))
			}
			g.Recipe = append(g.Recipe, slot)
		}
		st.Goods = append(st.Goods, g)
		rep.Imported++
	}

	// шаг 9: прогон валидаторов — предупреждения в отчёт (99a.2 §9)
	for _, w := range validate.Validate(st) {
		rep.Warnings = append(rep.Warnings, w.Message)
	}
	return rep
}

// slug — id внешнего компонента (спека 99a.2 §5.1): нормализованное имя,
// пробелы → дефис, кириллица допустима. Пример: «Питательная паста» →
// «питательная-паста».
func slug(name string) string {
	return strings.ReplaceAll(graph.NormalizeName(name), " ", "-")
}