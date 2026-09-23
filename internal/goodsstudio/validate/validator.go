// Package validate — валидаторы графа (спека 99a.1 §8): структурные
// проверки первой итерации (не качество и не перекосы — те в следующей).
package validate

import (
	"fmt"
	"strconv"

	"zorion/internal/goodsstudio/graph"
	"zorion/internal/goodsstudio/model"
)

// Warning — предупреждение валидатора.
type Warning struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// Validate прогоняет все валидаторы (спека 99a.1 §8).
func Validate(st *model.State) []Warning {
	var out []Warning

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

	// 2. Страховки: цикл и дубликат имени (невозможны по построению §6.2/§6.3)
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

	// 3. Товар без веса/объёма (спека 2026-09-20-фабрики §3.1, решение
	// 3b.6.4): NULL-каталог запрещён — механика грузов/трюма опирается на
	// данные каталога. С 2026-09-21 (Р2) значение есть всегда (`volume`/
	// `weight NOT NULL DEFAULT 1`) — проверка инертна, оставлена страховкой
	// (тот же класс, что cycle/duplicate_name: «невозможны по построению,
	// не чинить»). Ресурсы (kind=resource) пропускаются — сырьё не имеет
	// объёма/веса как товар (добывается платформой).
	for i := range st.Goods {
		g := &st.Goods[i]
		if g.Kind == model.KindResource {
			continue
		}
		if g.Volume == nil || g.Weight == nil {
			out = append(out, Warning{
				Code:    "missing_volume_weight",
				Message: fmt.Sprintf("Согласованный товар %s без веса/объёма (NULL-каталог запрещён)", g.Name),
			})
		}
	}

	// 4. «Товар без привязки» (спека 2026-09-21-рецепт-сущность §5):
	// товар (kind=good), чей рецепт не привязан ни к одной постройке
	// (producer_recipes) — «ничей» (спека фабрик §4.3); правится привязкой в
	// студии. Источник — st.Bindings (recipe_id/producer_type_id/good_id).
	// Носитель в UI — не этот warning: видимый маркер «не привязан» —
	// бейдж в справочнике (ТЗ @uidesigner §6.5), warning остаётся уровнем API.
	bound := make(map[string]bool, len(st.Bindings))
	for _, b := range st.Bindings {
		bound[strconv.FormatInt(b.GoodID, 10)] = true
	}
	for i := range st.Goods {
		g := &st.Goods[i]
		if g.Kind != model.KindGood {
			continue
		}
		if !bound[g.ID] {
			out = append(out, Warning{
				Code:    "unbound_recipe",
				Message: fmt.Sprintf("«Товар без привязки»: %s — рецепт не привязан ни к одной постройке", g.Name),
			})
		}
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