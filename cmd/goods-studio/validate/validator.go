// Package validate — валидаторы графа (спека 99a.1 §8): структурные
// проверки первой итерации (не качество и не перекосы — те в следующей).
package validate

import (
	"fmt"

	"zorion/cmd/goods-studio/graph"
	"zorion/cmd/goods-studio/model"
)

// Warning — предупреждение валидатора.
type Warning struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// Validate прогоняет все валидаторы (спека 99a.1 §8).
func Validate(st *model.State) []Warning {
	var out []Warning
	byID := graph.ByID(st.Goods)

	// входящая степень: сколько рецептов ссылаются на товар
	inDegree := make(map[string]int, len(st.Goods))
	for i := range st.Goods {
		for _, slot := range st.Goods[i].Recipe {
			if slot.GoodID != "" {
				inDegree[slot.GoodID]++
			}
		}
	}

	// 1. «Каких ресурсов нет» (ограничение первой итерации, решение №2):
	// листья графа (товары без заполненных слотов), на которые ссылаются
	// рецепты других товаров (входящая степень > 0), не покрытые каталогами
	// (слой 20 + витрина 111) → кандидаты в недостающие ресурсы.
	// Товар-верхушка с пустым рецептом (входящая степень 0) — лист-намерение,
	// в кандидаты не входит. Покрытие каталогами = импортированный ресурс
	// (kind=resource): товар с именем ресурса по построению §6.3 — ссылка
	// на ресурс, поэтому «не покрыт» ⟺ kind != resource.
	for i := range st.Goods {
		g := &st.Goods[i]
		if g.Kind == model.KindResource {
			continue
		}
		if hasFilledSlots(g.Recipe) {
			continue
		}
		if inDegree[g.ID] == 0 {
			continue
		}
		out = append(out, Warning{
			Code:    "missing_resource",
			Message: fmt.Sprintf("«Каких ресурсов нет»: %s — лист, на который ссылаются рецепты, но каталогами (слой 20 + витрина 111) не покрыт", g.Name),
		})
	}

	// 2. Неполные цепочки: согласованные товары с пустым рецептом (тир = 0).
	for i := range st.Goods {
		g := &st.Goods[i]
		if g.Status == model.StatusApproved && !hasFilledSlots(g.Recipe) {
			out = append(out, Warning{
				Code:    "incomplete_chain",
				Message: fmt.Sprintf("Неполная цепочка: согласованный товар %s без рецепта (тир 0)", g.Name),
			})
		}
	}

	// 3. Ссылки на не-согласованных: рецепт согласованного товара ссылается
	// на draft/excluded/banned составляющего (выгрузка будет с битой ссылкой).
	for i := range st.Goods {
		g := &st.Goods[i]
		if g.Status != model.StatusApproved {
			continue
		}
		for _, slot := range g.Recipe {
			if slot.GoodID == "" {
				continue
			}
			comp := byID[slot.GoodID]
			if comp == nil {
				continue
			}
			if comp.Status != model.StatusApproved && comp.Status != model.StatusResource {
				out = append(out, Warning{
					Code:    "non_approved_ref",
					Message: fmt.Sprintf("Ссылка на не-согласованного: %s ссылается на %s (%s)", g.Name, comp.Name, comp.Status),
				})
			}
		}
	}

	// 4. Страховки: цикл и дубликат имени (невозможны по построению §6.2/§6.3)
	// — проверка-страховка, не «чинить».
	if graph.HasCycle(st.Goods) {
		out = append(out, Warning{
			Code:    "cycle",
			Message: "Цикл в графе рецептов (невозможен по построению)",
		})
	}
	seen := make(map[string]string, len(st.Goods))
	for i := range st.Goods {
		norm := graph.NormalizeName(st.Goods[i].Name)
		if prev, ok := seen[norm]; ok {
			out = append(out, Warning{
				Code:    "duplicate_name",
				Message: fmt.Sprintf("Дубликат имени: %s и %s", prev, st.Goods[i].Name),
			})
		}
		seen[norm] = st.Goods[i].Name
	}
	return out
}

func hasFilledSlots(recipe []model.Slot) bool {
	for _, s := range recipe {
		if s.GoodID != "" {
			return true
		}
	}
	return false
}