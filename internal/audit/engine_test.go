package audit

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

type testView struct {
	id    string
	name  string
	value int
}

func (v *testView) GetID() string         { return v.id }
func (v *testView) GetName() string       { return v.name }
func (v *testView) GetContextID() string  { return "" }
func (v *testView) GetEntityType() string { return "test" }

// issueForID — правило, выдающее ровно одну проблему для своей сущности.
func issueForID(code, severity string) Rule[testView] {
	return Rule[testView]{
		Check: func(v *testView) []Issue {
			return []Issue{{EntityID: v.id, Code: code, Severity: severity}}
		},
	}
}

// okRule — правило без проблем.
func okRule() Rule[testView] {
	return Rule[testView]{Check: func(*testView) []Issue { return nil }}
}

// ————— ПУСТЫЕ ВХОДЫ —————

func TestRunEmpty(t *testing.T) {
	res := Run[testView]("test", nil, nil)

	// Пустой аудит: нулевые счётчики, тип сохраняется.
	assert.Equal(t, "test", res.EntityType)
	assert.Equal(t, 0, res.TotalEntities)
	assert.Equal(t, 0, res.TotalIssues)
	assert.Equal(t, 0, res.EntitiesWithIssue)
	assert.Empty(t, res.IssuesByCode)
	assert.Empty(t, res.IssuesBySeverity)
	assert.False(t, res.Truncated)
	assert.Empty(t, res.SampleIssues)
}

func TestRunNoRulesNoIssues(t *testing.T) {
	views := []*testView{{id: "v1"}, {id: "v2"}}
	res := Run[testView]("test", views, nil)

	assert.Equal(t, 2, res.TotalEntities)
	assert.Equal(t, 0, res.TotalIssues)
}

// ————— АГРЕГАЦИЯ —————

func TestRunAggregation(t *testing.T) {
	views := []*testView{{id: "v1"}, {id: "v2"}}
	// Два правила применяются к каждой сущности.
	rules := []Rule[testView]{
		issueForID("sum_not_100", SeverityHigh),
		issueForID("negative_share", SeverityMedium),
	}

	res := Run[testView]("test", views, rules)

	// Для 2 сущностей × 2 правила ожидаем 4 проблемы, по 2 на код и уровень.
	assert.Equal(t, 2, res.TotalEntities)
	assert.Equal(t, 2, res.EntitiesWithIssue)
	assert.Equal(t, 4, res.TotalIssues)
	assert.Equal(t, 2, res.IssuesByCode["sum_not_100"])
	assert.Equal(t, 2, res.IssuesByCode["negative_share"])
	assert.Equal(t, 2, res.IssuesBySeverity[SeverityHigh])
	assert.Equal(t, 2, res.IssuesBySeverity[SeverityMedium])
	assert.Len(t, res.SampleIssues, 4)
	assert.False(t, res.Truncated)
}

func TestRunEntitiesWithIssueCountsUnique(t *testing.T) {
	views := []*testView{{id: "v1"}, {id: "v2"}, {id: "v3"}}
	// v1 ломает оба правила, v2 — одно, v3 — чистая.
	rules := []Rule[testView]{
		{Check: func(v *testView) []Issue {
			if v.id == "v1" || v.id == "v2" {
				return []Issue{{EntityID: v.id, Code: "a", Severity: SeverityLow}}
			}
			return nil
		}},
		{Check: func(v *testView) []Issue {
			if v.id == "v1" {
				return []Issue{{EntityID: v.id, Code: "b", Severity: SeverityMedium}}
			}
			return nil
		}},
	}

	res := Run[testView]("test", views, rules)

	// Сущностей с проблемами — 2 (v1, v2), всего проблем — 3.
	assert.Equal(t, 3, res.TotalIssues)
	assert.Equal(t, 2, res.EntitiesWithIssue)
}

// ————— ЛИМИТ ПРИМЕРОВ —————

func TestRunSampleLimit(t *testing.T) {
	views := []*testView{{id: "v1"}}
	bulk := make([]Issue, 0, SampleLimit+50)
	for i := 0; i < SampleLimit+50; i++ {
		bulk = append(bulk, Issue{EntityID: "v1", Code: "many", Severity: SeverityLow})
	}
	rules := []Rule[testView]{{Check: func(*testView) []Issue { return bulk }}}

	res := Run[testView]("test", views, rules)

	// Счётчики полные, но примеров не больше SampleLimit.
	assert.Equal(t, SampleLimit+50, res.TotalIssues)
	assert.Equal(t, SampleLimit+50, res.IssuesByCode["many"])
	assert.Len(t, res.SampleIssues, SampleLimit)
	assert.True(t, res.Truncated)
}

// ————— УСТОЙЧИВОСТЬ —————

func TestRunPanicRuleIsolated(t *testing.T) {
	views := []*testView{{id: "v1"}}
	panicking := Rule[testView]{Check: func(*testView) []Issue { panic("boom") }}
	ok := issueForID("fine", SeverityLow)

	res := Run[testView]("test", views, []Rule[testView]{panicking, ok})

	// Паника правила не прерывает аудит: правильное правило отработало.
	assert.Equal(t, 1, res.TotalIssues)
	assert.Equal(t, 1, res.IssuesByCode["fine"])
	assert.Equal(t, 1, res.EntitiesWithIssue)
}

func TestRunNilCheckSkipped(t *testing.T) {
	views := []*testView{{id: "v1"}}
	res := Run[testView]("test", views, []Rule[testView]{{Check: nil}, okRule()})

	assert.Equal(t, 0, res.TotalIssues)
	assert.Equal(t, 0, res.EntitiesWithIssue)
}