package catalog

import (
	"encoding/json"
	"fmt"
	"strings"

	"zorion/internal/goodsstudio/graph"
	"zorion/internal/goodsstudio/model"
	"zorion/internal/goodsstudio/validate"
)

// TreesCatalog — формат goods_data/trees_t8.json (schema_version 2): полные
// деревья производства Т8. Файл только читается, не модифицируется.
// Деревья вводятся как ОДНА СЕТЬ: общая база (узлы s_*, продублированы в
// обоих деревьях с тем же id) вводится один раз (дедуп по id).
type TreesCatalog struct {
	SchemaVersion int             `json:"schema_version"`
	Catalog       string          `json:"catalog"`
	GeneratedAt   string          `json:"generated_at"`
	Rework        string          `json:"rework"` // строка-комментарий, не импортируется
	Trees         []GoodTree      `json:"trees"`
	Pool          json.RawMessage `json:"pool"`    // пул на будущее, не импортируется
	Summary       json.RawMessage `json:"summary"` // справочное, не импортируется
}

// GoodTree — одно дерево производства (корень + все узлы, включая корень).
type GoodTree struct {
	Root            TreeNode   `json:"root"`
	Nodes           []TreeNode `json:"nodes"`
	LeavesResources []string   `json:"leaves_resources"` // справочный список имён ресурсов, не импортируется
}

// TreeNode — узел дерева: товар с рецептом (constituents).
type TreeNode struct {
	ID           string            `json:"id"`
	Name         string            `json:"name"`
	Tier         int               `json:"tier"` // тир из файла (1..8); сверяется с вычисленным
	Category     string            `json:"category"`
	Constituents []TreeConstituent `json:"constituents"`
}

// TreeConstituent — составляющая узла: good (с ref на id узла) или resource
// (лист, ref нет — резолв по имени на res:*).
type TreeConstituent struct {
	Ref      string `json:"ref"` // id узла good; у ресурсов пуст
	Name     string `json:"name"`
	Type     string `json:"type"`     // "resource" | "good"
	Quantity int    `json:"quantity"` // единиц составляющей; 0 = не задано → 1
}

// ParseTreesCatalog читает и валидирует trees_t8.json (schema_version 2).
// Ошибка — перечень всех проблем (400, импорт не идёт).
func ParseTreesCatalog(data []byte) (*TreesCatalog, error) {
	var tc TreesCatalog
	if err := json.Unmarshal(data, &tc); err != nil {
		return nil, fmt.Errorf("невалидный JSON: %v", err)
	}
	var problems []string
	if tc.SchemaVersion != 2 {
		problems = append(problems, fmt.Sprintf("неподдерживаемая версия каталога: %d (ожидается 2)", tc.SchemaVersion))
	}
	if len(tc.Trees) == 0 {
		problems = append(problems, "каталог пуст: trees не содержит деревьев")
	}
	// id узла → первое определение (для сверки дублей общей базы s_*)
	firstByID := make(map[string]TreeNode, 64)
	for ti, tree := range tc.Trees {
		if strings.TrimSpace(tree.Root.Name) == "" {
			problems = append(problems, fmt.Sprintf("дерево %d: пустое имя корня", ti+1))
		}
		for ni, node := range tree.Nodes {
			id := strings.TrimSpace(node.ID)
			if id == "" {
				problems = append(problems, fmt.Sprintf("дерево %d, узел %d: пустой id", ti+1, ni+1))
				continue
			}
			if prev, ok := firstByID[id]; ok {
				// общая база s_* дублируется в деревьях ДОСЛОВНО (дедуп на
				// импорте); тот же id с другим содержимым — ошибка каталога
				if !sameTreeNode(prev, node) {
					problems = append(problems, fmt.Sprintf("дубликат id узла: %s (разное содержимое)", id))
				}
			} else {
				firstByID[id] = node
			}
			if strings.TrimSpace(node.Name) == "" {
				problems = append(problems, fmt.Sprintf("узел %s: пустое имя", id))
			}
			if len(node.Constituents) == 0 {
				problems = append(problems, fmt.Sprintf("узел %s: пустой состав (constituents)", id))
			}
			for j, c := range node.Constituents {
				if strings.TrimSpace(c.Name) == "" {
					problems = append(problems, fmt.Sprintf("узел %s: составляющая %d: пустое имя", id, j+1))
				}
				if c.Type != "resource" && c.Type != "good" {
					problems = append(problems, fmt.Sprintf("узел %s: составляющая %q: невалидный тип %q (ожидается resource|good)", id, c.Name, c.Type))
				}
			}
		}
	}
	if len(problems) > 0 {
		return nil, fmt.Errorf("каталог невалиден:\n- %s", strings.Join(problems, "\n- "))
	}
	return &tc, nil
}

// sameTreeNode — дословное равенство узлов (для проверки дублей общей базы).
func sameTreeNode(a, b TreeNode) bool {
	if a.ID != b.ID || a.Name != b.Name || a.Tier != b.Tier || a.Category != b.Category {
		return false
	}
	if len(a.Constituents) != len(b.Constituents) {
		return false
	}
	for i := range a.Constituents {
		if a.Constituents[i] != b.Constituents[i] {
			return false
		}
	}
	return true
}

// ApplyTreesCatalog — полная замена товаров студии деревьями Т8 (schema 2).
// Ресурсы (kind=resource) сохраняются со всеми полями (статусы/баны);
// товары (kind=good) сносятся; категории маппятся по нормализованному имени
// (новые — создаются); ВСЕ узлы деревьев (верхушки, промежуточные, база)
// импортируются как товары approved/kind=good/source=import; общая база
// (s_*) дедуплицируется по id (входит один раз); рецепт узла — слот на
// каждую составляющую (quantity из файла или 1), good резолвится по ref/id,
// resource — по имени (res:*); битый ref → пустой слот + предупреждение;
// вычисленный тир сверяется с файловым (предупреждение, не ошибка);
// прогон валидаторов — в отчёт. Мутирует st.
func ApplyTreesCatalog(st *model.State, tc *TreesCatalog) *ImportReport {
	rep := &ImportReport{Trees: len(tc.Trees)}

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
	// нормализованному имени; не найденная — создаётся
	catByNorm := make(map[string]string, len(st.Categories))
	for _, c := range st.Categories {
		catByNorm[graph.NormalizeName(c.Name)] = c.ID
	}
	catID := func(name string) string {
		norm := graph.NormalizeName(name)
		id, ok := catByNorm[norm]
		if !ok {
			id = model.NextCategoryID(st.Categories)
			st.Categories = append(st.Categories, model.Category{ID: id, Name: name})
			catByNorm[norm] = id
			rep.CategoriesCreated++
		}
		return id
	}

	// шаг 5: все узлы деревьев как товары — дедуп общей базы по id
	// (s_* входит один раз); верхушки и промежуточные тоже импортируются
	created := make(map[string]bool, 64)
	for _, tree := range tc.Trees {
		for _, node := range tree.Nodes {
			if created[node.ID] {
				rep.BaseDeduplicated++
				continue
			}
			created[node.ID] = true
			st.Goods = append(st.Goods, model.Good{
				ID:        node.ID,
				Name:      node.Name,
				Category:  catID(node.Category),
				Status:    model.StatusApproved, // сразу согласовано — поедет в экспорт
				Kind:      model.KindGood,
				Source:    model.SourceImport,
				Recipe:    make([]model.Slot, 0, len(node.Constituents)),
				CreatedAt: model.NowISO(),
			})
			rep.NodesCreated++
		}
	}
	rep.Imported = rep.NodesCreated

	// шаг 6: рецепты — слот на каждую составляющую; good по ref/id,
	// resource по имени (res:*); битый ref → пустой слот + предупреждение.
	// processed — «рецепт уже заполнен»: дубль общей базы (s_* во втором
	// дереве) пропускается, иначе рецепт заполнился бы повторно (двойные
	// слоты). created (шаг 5) для этого не годится: у дубля он уже true.
	byID := graph.ByID(st.Goods)
	processed := make(map[string]bool, len(created))
	for _, tree := range tc.Trees {
		for _, node := range tree.Nodes {
			if processed[node.ID] {
				continue // дубль общей базы — рецепт уже заполнен при первом вхождении
			}
			processed[node.ID] = true
			g := byID[node.ID]
			for _, c := range node.Constituents {
				slot := model.Slot{Quantity: 1}
				if c.Quantity > 1 {
					slot.Quantity = c.Quantity
				}
				switch c.Type {
				case "good":
					if c.Ref != "" && byID[c.Ref] != nil {
						slot.GoodID = c.Ref
					} else {
						rep.Warnings = append(rep.Warnings, fmt.Sprintf("не найден компонент по ref: %s (узел %s)", c.Ref, node.ID))
					}
				case "resource":
					if comp := graph.FindByName(c.Name, st.Goods); comp != nil {
						slot.GoodID = comp.ID
					} else {
						rep.Warnings = append(rep.Warnings, fmt.Sprintf("не найден ресурс по имени: %s (узел %s)", c.Name, node.ID))
					}
				}
				g.Recipe = append(g.Recipe, slot)
			}
		}
	}

	// шаг 7: сверка тиров — вычисленный (1 + max тиров составляющих) против
	// тира из файла; расхождение — предупреждение, не ошибка. Только для
	// обработанных узлов (дубли базы не дают двойных предупреждений).
	for _, tree := range tc.Trees {
		for _, node := range tree.Nodes {
			if !processed[node.ID] {
				continue
			}
			if t := graph.Tier(byID[node.ID], byID); t != node.Tier {
				rep.Warnings = append(rep.Warnings, fmt.Sprintf("тир узла %s: вычисленный %d ≠ из файла %d", node.ID, t, node.Tier))
			}
		}
	}

	// шаг 8: прогон валидаторов — предупреждения в отчёт
	for _, w := range validate.Validate(st) {
		rep.Warnings = append(rep.Warnings, w.Message)
	}
	return rep
}
