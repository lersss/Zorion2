// internal/audit/engine.go
package audit

import (
	"log"
	"time"
)

// Run — обобщённый движок аудита.
//
// Принимает:
//   - entityType — тип сущностей ("planet", "star", "faction").
//   - views — список представлений сущностей.
//   - rules — список правил, применимых к этим сущностям.
//
// Возвращает агрегированный AuditResult.
//
// Движок не знает, что именно проверяется: планеты, звёзды, фракции —
// это определяется типом T и набором правил.
//
// Пример использования:
//
//	rules := planet.AllRules()
//	views := planet.ParseAll(rows)
//	result := audit.Run("planet", views, rules)
func Run[T any](
	entityType string,
	views []*T,
	rules []Rule[T],
) *AuditResult {
	start := time.Now()

	result := &AuditResult{
		EntityType:       entityType,
		TotalEntities:    len(views),
		IssuesByCode:     make(map[string]int),
		IssuesBySeverity: make(map[string]int),
		HighIssuesByCode: make(map[string]int),
		SampleIssues:     make([]Issue, 0, SampleLimit),
	}

	if len(views) == 0 || len(rules) == 0 {
		result.DurationMs = time.Since(start).Milliseconds()
		return result
	}

	entitiesWithIssues := make(map[string]bool)
	entitiesWithHigh := make(map[string]bool)

	for _, view := range views {
		for _, rule := range rules {
			issues := safeCheck(rule, view)
			for _, issue := range issues {
				result.IssuesByCode[issue.Code]++
				result.IssuesBySeverity[issue.Severity]++
				result.TotalIssues++
				entitiesWithIssues[issue.EntityID] = true

				if issue.Severity == SeverityHigh {
					// High — глобальное противоречие («невозможная» планета).
					entitiesWithHigh[issue.EntityID] = true
					result.HighIssuesByCode[issue.Code]++
				}

				if len(result.SampleIssues) < SampleLimit {
					result.SampleIssues = append(result.SampleIssues, issue)
				}
			}
		}
	}

	result.EntitiesWithIssue = len(entitiesWithIssues)
	result.EntitiesWithHighIssue = len(entitiesWithHigh)
	result.Truncated = result.TotalIssues > SampleLimit
	result.DurationMs = time.Since(start).Milliseconds()

	log.Printf("🔍 audit[%s]: %d entities, %d with issues, %d total issues, %d ms",
		entityType, result.TotalEntities, result.EntitiesWithIssue,
		result.TotalIssues, result.DurationMs)

	return result
}

// safeCheck — обёртка над rule.Check с защитой от паник.
// Если одно правило упадёт, аудит не остановится — просто пропустит его
// для этой сущности и залогирует.
func safeCheck[T any](rule Rule[T], view *T) (issues []Issue) {
	defer func() {
		if rec := recover(); rec != nil {
			log.Printf("🔥 panic in audit rule: %v", rec)
		}
	}()
	if rule.Check == nil {
		return nil
	}
	return rule.Check(view)
}