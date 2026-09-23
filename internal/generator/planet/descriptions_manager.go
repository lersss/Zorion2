// internal/generator/planet/descriptions_manager.go
package planet

import (
	"log"
	"strings"
)

// ==================== ГЛОБАЛЬНЫЙ МЕНЕДЖЕР ====================

var globalDescriptions *descriptionsManager

// LoadDescriptionsGlobal — инициализация глобального менеджера.
func LoadDescriptionsGlobal(baseDir string) error {
	m, err := LoadDescriptions(baseDir)
	if err != nil {
		return err
	}
	globalDescriptions = m
	return nil
}

// ==================== ТОЧКА ВХОДА ====================

// GenerateDescription — собирает описание планеты: зачин + концовка.
// Если для типа нет подходящих записей — возвращает fallback.
func GenerateDescription(ctx DescriptionContext) string {
	if globalDescriptions == nil {
		return fallbackDescription(ctx.PlanetID)
	}
	return globalDescriptions.generate(ctx)
}

// generate — внутренняя реализация.
func (m *descriptionsManager) generate(ctx DescriptionContext) string {
	if ctx.PlanetID == "" || ctx.Type == "" {
		return fallbackDescription(ctx.PlanetID)
	}

	folder := typeToFolder(ctx.Type)
	if folder == "" {
		log.Printf(
			"descriptions: неизвестный тип %q для планеты %s",
			ctx.Type, ctx.PlanetID,
		)
		return fallbackDescription(ctx.PlanetID)
	}

	m.mu.RLock()
	_, hasOpenings := m.openings[folder]
	_, hasClosings := m.closings[folder]
	m.mu.RUnlock()
	if !hasOpenings && !hasClosings {
		// Тип зарегистрирован, но папки описаний ещё нет (мини-нептун, спека
		// 2026-09-23 §11.2): нейтральный fallback БЕЗ лога на каждую планету.
		return fallbackDescription(ctx.PlanetID)
	}

	tags := computeTags(ctx)

	m.mu.RLock()
	defer m.mu.RUnlock()

	opening, okOpen := m.pickOpening(ctx.PlanetID, folder, tags)
	closing, okClose := m.pickClosing(ctx.PlanetID, folder, tags)

	if !okOpen && !okClose {
		log.Printf(
			"descriptions: fallback для планеты %s (тип %s, папка %s, тегов=%d)",
			ctx.PlanetID, ctx.Type, folder, len(tags),
		)
		return fallbackDescription(ctx.PlanetID)
	}

	return joinDescription(opening, closing)
}

// ==================== СКЛЕЙКА ====================

func joinDescription(opening, closing string) string {
	switch {
	case opening == "" && closing == "":
		return ""
	case opening == "":
		return closing
	case closing == "":
		return opening
	default:
		return opening + descriptionSeparator + closing
	}
}

// ==================== FALLBACK ====================

var fallbackVariants = []string{
	"Ничем не примечательная планета. Ни ресурсов, ни жизни, ни истории — только камень и время.",

	"Обычный мир без ярких особенностей. Таких в галактике много, и большинство экспедиций проходит мимо, не задерживаясь.",

	"Планета не запомнилась ничем. Ровная, спокойная, предсказуемая — из тех, что служат фоном для более интересных мест.",

	"Здесь нечего искать. Мир, который существует сам по себе и не предлагает ничего, кроме самого факта своего существования.",

	"Непримечательный мир. Ни воды, ни биосферы, ни следов разума — только порода, пыль и медленное время.",

	"Планета без истории. Она была здесь до нас и будет после — тихая, серая, никому не нужная.",

	"Один из тех миров, что редко попадают на карты. Не потому что опасны или недоступны, а потому что нечем зацепиться.",
}

func fallbackDescription(planetID string) string {
	if len(fallbackVariants) == 0 {
		return ""
	}
	if planetID == "" {
		return fallbackVariants[0]
	}
	idx := hashIndex(planetID+":fallback", len(fallbackVariants))
	return fallbackVariants[idx]
}

// ==================== ДИАГНОСТИКА ====================

func DescriptionsStats() map[string]descriptionsStats {
	if globalDescriptions == nil {
		return nil
	}
	globalDescriptions.mu.RLock()
	defer globalDescriptions.mu.RUnlock()

	out := make(map[string]descriptionsStats, len(globalDescriptions.stats))
	for k, v := range globalDescriptions.stats {
		out[k] = v
	}
	return out
}

// ==================== УТИЛИТЫ ====================

func trimDescription(s string) string {
	s = strings.TrimSpace(s)
	for strings.Contains(s, "\n\n\n") {
		s = strings.ReplaceAll(s, "\n\n\n", "\n\n")
	}
	return s
}