package ai

import (
	"fmt"
	"strconv"

	"zorion/internal/goodsstudio/graph"
	"zorion/internal/goodsstudio/model"
)

// Proposal — предложение ИИ для одного пустого слота (спека
// переноса-студии-товаров-iterC §5.3): показывается в попапе «Предложения
// ИИ», применяется только принятое (двухфазный fill «предложи → подтверди»,
// решение создателя 2026-09-20).
type Proposal struct {
	Slot          int    `json:"slot"`           // pos пустого слота (0-based)
	Name          string `json:"name"`           // имя товара/ресурса
	Category      string `json:"category"`       // как ИИ назвал категорию (для пометки «не найдена»)
	CategoryID    int64  `json:"category_id"`    // резолв если валидна, иначе 0
	CategoryValid bool   `json:"category_valid"` // производное от резолва
	Reason        string `json:"reason"`         // ИИ-обоснование (тултип)
	Kind          string `json:"kind"`           // "new" — создать новый товар / "link" — заполнить готовым
	LinkID        string `json:"link_id,omitempty"`
	LinkName      string `json:"link_name,omitempty"`
}

// ProposalItem — принятый пункт попапа для применения (спека iterC §5.4):
// финальная категория уже выбрана попапом — резолв категории на apply не
// нужен; Kind нужен для С1 (link-цель исчезла → дроп, НЕ перевод в kind=new).
type ProposalItem struct {
	Slot       int
	Name       string
	CategoryID string // строка id выбранной категории (для kind=new)
	Reason     string
	Kind       string // "new" | "link"
}

// BuildProposals — разбор ответа ИИ в предложения (фаза 1, спека iterC
// §5.4): для каждого компонента (в порядке пустых слотов) — лишние
// отбрасываются («больше запрошенного»), меньше — информационно; резолв
// имени (NameIndex): найдено → kind=link, не найдено → kind=new (категория:
// resolveCategoryID → category_id/category_valid); дропы на фазе разбора
// (не показываются в попапе, строки в отчёт): бан/исключённое, ресурс в
// слот без галки, цикл для link (новый товар — лист, цикла дать не может).
// Возвращает actionable-предложения + отчёт дропов. Чистая функция.
func BuildProposals(st *model.State, goodID string, comps []Component) ([]Proposal, []string) {
	var report []string
	gi := indexOfGood(st.Goods, goodID)
	if gi < 0 {
		return nil, append(report, "товар не найден")
	}
	empty := emptySlotIndices(st.Goods[gi].Recipe)
	k := len(empty)
	if len(comps) > k {
		report = append(report, fmt.Sprintf("ИИ вернул больше запрошенного: %d лишних — пропущены", len(comps)-k))
		comps = comps[:k]
	} else if len(comps) < k {
		report = append(report, fmt.Sprintf("ИИ вернул меньше запрошенного: %d из %d — остальные слоты пустые", len(comps), k))
	}

	byName := graph.NameIndex(st.Goods)
	var proposals []Proposal
	for i, comp := range comps {
		name := graph.NormalizeName(comp.Name)
		if name == "" {
			continue
		}
		slot := empty[i]
		// дропы на фазе разбора (не показываются в попапе, строки в отчёт)
		if idx, ok := byName[name]; ok {
			existing := &st.Goods[idx]
			if existing.Status == model.StatusBanned || existing.Status == model.StatusExcluded {
				report = append(report, fmt.Sprintf("ИИ предложил %s: %s — пропущено", statusWord(existing.Status), existing.Name))
				continue
			}
			if existing.Kind == model.KindResource && !st.Goods[gi].Recipe[slot].AllowResource {
				report = append(report, fmt.Sprintf("ресурс %s не разрешён для этого слота — включи галку \"заполнять ресурсом\"", existing.Name))
				continue
			}
			byID := graph.ByID(st.Goods)
			if graph.WouldCreateCycle(existing.ID, goodID, byID) {
				report = append(report, fmt.Sprintf("ИИ предложил цикл: %s — пропущено", comp.Name))
				continue
			}
			proposals = append(proposals, Proposal{
				Slot: slot, Name: existing.Name, Reason: comp.Reason,
				Kind: "link", LinkID: existing.ID, LinkName: existing.Name,
			})
			continue
		}
		// не найдено — kind=new: категория резолвится (валидность — пометка
		// в попапе, выбор категории — за пользователем, решение 2026-09-20)
		catID, catValid := resolveCategoryID(comp.Category, st.Categories)
		proposals = append(proposals, Proposal{
			Slot: slot, Name: comp.Name, Category: comp.Category,
			CategoryID: catID, CategoryValid: catValid,
			Reason: comp.Reason, Kind: "new",
		})
	}
	return proposals, report
}

// ApplyProposals — применение принятых пунктов (фаза 2, спека iterC §5.4):
// per-slot — заполняется именно item.Slot, не «первые пустые по порядку».
// Валидации на свежем снимке (каждая — дроп пункта + строка в отчёт, дропы
// в applied НЕ входят): С3 слот существует и пуст; С1 link-цель найдена
// (исчезла → дроп, НЕ перевод в kind=new); С2 категория kind=new существует
// и kind=good; бан/исключённое; ресурс в слот без галки; цикл. Мутирует st
// (только: добавляет новые товары в конец, заполняет конкретные слоты
// целевого товара). Возвращает число фактически записанных пунктов + отчёт.
func ApplyProposals(st *model.State, goodID string, items []ProposalItem) (int, []string) {
	var report []string
	gi := indexOfGood(st.Goods, goodID)
	if gi < 0 {
		return 0, append(report, "товар не найден")
	}
	if len(emptySlotIndices(st.Goods[gi].Recipe)) == 0 {
		return 0, append(report, "нет пустых слотов")
	}
	byName := graph.NameIndex(st.Goods)
	applied := 0
	for _, item := range items {
		// С3 — слот существует в recipe целевого товара и пуст
		if item.Slot < 0 || item.Slot >= len(st.Goods[gi].Recipe) || st.Goods[gi].Recipe[item.Slot].GoodID != "" {
			report = append(report, fmt.Sprintf("слот %d занят/удалён — пропущено", item.Slot))
			continue
		}
		name := graph.NormalizeName(item.Name)
		if name == "" {
			report = append(report, "пустое имя — пропущено")
			continue
		}
		if item.Kind == "link" {
			// С1 — link-цель: имя должно находиться (товар удалён/переименован
			// между фазами → дроп; НЕ переводить в kind=new — у link-пункта
			// категории нет → FK-ошибка)
			idx, ok := byName[name]
			if !ok {
				report = append(report, fmt.Sprintf("ссылка исчезла (товар удалён/переименован): %s — пропущено", item.Name))
				continue
			}
			existing := &st.Goods[idx]
			if existing.Status == model.StatusBanned || existing.Status == model.StatusExcluded {
				report = append(report, fmt.Sprintf("ИИ предложил %s: %s — пропущено", statusWord(existing.Status), existing.Name))
				continue
			}
			if existing.Kind == model.KindResource && !st.Goods[gi].Recipe[item.Slot].AllowResource {
				report = append(report, fmt.Sprintf("ресурс %s не разрешён для этого слота — включи галку \"заполнять ресурсом\"", existing.Name))
				continue
			}
			byID := graph.ByID(st.Goods)
			if graph.WouldCreateCycle(existing.ID, goodID, byID) {
				report = append(report, fmt.Sprintf("ИИ предложил цикл: %s — пропущено", existing.Name))
				continue
			}
			st.Goods[gi].Recipe[item.Slot].GoodID = existing.ID
			st.Goods[gi].Recipe[item.Slot].Reason = item.Reason
			applied++
			continue
		}
		// kind=new: имя нашлось — ссылка на существующий (безопасно, категория
		// игнорируется); не нашлось — создание нового товара
		if idx, ok := byName[name]; ok {
			existing := &st.Goods[idx]
			if existing.Status == model.StatusBanned || existing.Status == model.StatusExcluded {
				report = append(report, fmt.Sprintf("ИИ предложил %s: %s — пропущено", statusWord(existing.Status), existing.Name))
				continue
			}
			if existing.Kind == model.KindResource && !st.Goods[gi].Recipe[item.Slot].AllowResource {
				report = append(report, fmt.Sprintf("ресурс %s не разрешён для этого слота — включи галку \"заполнять ресурсом\"", existing.Name))
				continue
			}
			byID := graph.ByID(st.Goods)
			if graph.WouldCreateCycle(existing.ID, goodID, byID) {
				report = append(report, fmt.Sprintf("ИИ предложил цикл: %s — пропущено", existing.Name))
				continue
			}
			st.Goods[gi].Recipe[item.Slot].GoodID = existing.ID
			st.Goods[gi].Recipe[item.Slot].Reason = item.Reason
			applied++
			continue
		}
		// С2 — категория: существует и kind=good в текущем снимке (DeleteCategory
		// под lock мог убрать её после выбора в попапе)
		if !categoryExistsGood(st.Categories, item.CategoryID) {
			report = append(report, fmt.Sprintf("категория удалена: %s — пропущено", item.Name))
			continue
		}
		newGood := model.Good{
			ID:        model.NextGoodID(st.Goods),
			Name:      item.Name,
			Category:  item.CategoryID,
			Status:    model.StatusDraft,
			Kind:      model.KindGood,
			Source:    model.SourceAI,
			Recipe:    []model.Slot{},
			CreatedAt: model.NowISO(),
		}
		st.Goods = append(st.Goods, newGood)
		byName[name] = len(st.Goods) - 1
		st.Goods[gi].Recipe[item.Slot].GoodID = newGood.ID
		st.Goods[gi].Recipe[item.Slot].Reason = item.Reason
		applied++
	}
	return applied, report
}

// resolveCategoryID — категория из ответа ИИ (спека iterC §5.4): точный ID →
// нормализованное совпадение имени → (0, false). Валидность — пометка в
// попапе, выбор категории — за пользователем (решение создателя 2026-09-20).
func resolveCategoryID(compCat string, cats []model.Category) (int64, bool) {
	if compCat == "" {
		return 0, false
	}
	for _, c := range cats {
		if c.ID == compCat {
			id, _ := strconv.ParseInt(c.ID, 10, 64)
			return id, true
		}
	}
	norm := graph.NormalizeName(compCat)
	for _, c := range cats {
		if graph.NormalizeName(c.Name) == norm {
			id, _ := strconv.ParseInt(c.ID, 10, 64)
			return id, true
		}
	}
	return 0, false
}

// categoryExistsGood — категория существует и kind=good (С2, спека iterC
// §5.4): новые товары от ИИ — только в товарные категории (category_id
// NOT NULL, инвариант iterA №3).
func categoryExistsGood(cats []model.Category, id string) bool {
	for _, c := range cats {
		if c.ID == id {
			return c.Kind == model.KindGood
		}
	}
	return false
}