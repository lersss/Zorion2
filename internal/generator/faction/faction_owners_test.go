// Тесты прохода владельцев новых поселений (спека
// 2026-09-24-постройка-структур-на-планете §3.4, Р8/Г1): владелец = фракция
// расы поселения, скоуп — только поселения текущей генерации (created_at >=
// since); нет метки генерации → UPDATE не выполняется (легаси не трогаем).
package faction

import (
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
)

// ownersSQLPattern — регулярка UPDATE владельцев: owner_type='faction',
// owner_id = factions.id, скоуп owner_id IS NULL + created_at >= $1 + раса.
const ownersSQLPattern = `UPDATE settlements s[\s\S]*SET owner_type = 'faction', owner_id = f\.id[\s\S]*WHERE s\.owner_id IS NULL[\s\S]*s\.created_at >= \$1[\s\S]*s\.race_id IS NOT NULL AND s\.race_id <> ''[\s\S]*f\.race_id = s\.race_id`

// TestEnsureSettlementOwnersScopedBySince — метка есть: UPDATE идёт с $1=since,
// возвращает число обновлённых; SQL ограничен текущей генерацией.
func TestEnsureSettlementOwnersScopedBySince(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	since := time.Date(2026, 9, 24, 10, 0, 0, 0, time.UTC)
	mock.ExpectExec(ownersSQLPattern).WithArgs(since).WillReturnResult(sqlmock.NewResult(0, 7))

	n, err := NewGenerator(db, 1).EnsureSettlementOwners(since)
	require.NoError(t, err)
	require.Equal(t, 7, n, "число обновлённых поселений = RowsAffected")
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestEnsureSettlementOwnersNoMarkerNoUpdate — нулевой since («метки нет») →
// UPDATE не выполняется: легаси-владельцы не появляются (Г1).
func TestEnsureSettlementOwnersNoMarkerNoUpdate(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	// Ни одного ожидания: любой запрос = неожиданный (ExpectationsWereMet упадёт).
	n, err := NewGenerator(db, 1).EnsureSettlementOwners(time.Time{})
	require.NoError(t, err)
	require.Equal(t, 0, n)
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestGenerationStartNoMarkerIsNormal — нет ключа (sql.ErrNoRows) — норма:
// ok=false, ошибки нет, проход владельцев не выполняется (Г1).
func TestGenerationStartNoMarkerIsNormal(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectQuery(generationStartPattern).WillReturnRows(sqlmock.NewRows([]string{"payload"}))

	_, ok, err := NewGenerator(db, 1).generationStart()
	require.NoError(t, err, "отсутствие метки — не ошибка")
	require.False(t, ok)
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestGenerationStartDBErrorPropagates — реальный сбой БД (не ErrNoRows) —
// ошибка, а не тихий no-op: иначе сбой молча гасит проход владельцев.
func TestGenerationStartDBErrorPropagates(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	boom := errors.New("db down")
	mock.ExpectQuery(generationStartPattern).WillReturnError(boom)

	_, ok, err := NewGenerator(db, 1).generationStart()
	require.ErrorIs(t, err, boom)
	require.False(t, ok)
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestGenerateFactionsPropagatesMarkerDBError — сбой чтения метки генерации в
// GenerateFactions возвращается наружу (проход владельцев не гаснет молча).
func TestGenerateFactionsPropagatesMarkerDBError(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectQuery(raceCandidatesPattern).WillReturnRows(
		raceCandidateRows().AddRow("humans", "p1", 100, "Аврора", `{}`))
	mock.ExpectQuery(existingRacesPattern).WillReturnRows(sqlmock.NewRows([]string{"race_id"}))
	expectFactionInsert(mock, "humans", "humans", "p1")
	mock.ExpectExec(capitalsSQLPattern).WillReturnResult(sqlmock.NewResult(0, 1))
	boom := errors.New("db down")
	mock.ExpectQuery(generationStartPattern).WillReturnError(boom)

	_, _, err = NewGenerator(db, 7).GenerateFactions()
	require.ErrorIs(t, err, boom)
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestGenerateFactionsRunsOwnerPass — при метке генерации в generation_config
// GenerateFactions после столиц выполняет проход владельцев.
func TestGenerateFactionsRunsOwnerPass(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectQuery(raceCandidatesPattern).WillReturnRows(
		raceCandidateRows().AddRow("humans", "p1", 100, "Аврора", `{}`))
	mock.ExpectQuery(existingRacesPattern).WillReturnRows(sqlmock.NewRows([]string{"race_id"}))
	expectFactionInsert(mock, "humans", "humans", "p1")
	mock.ExpectExec(capitalsSQLPattern).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(generationStartPattern).WillReturnRows(
		sqlmock.NewRows([]string{"payload"}).AddRow([]byte(`"2026-09-24T10:00:00Z"`)))
	mock.ExpectExec(ownersSQLPattern).
		WithArgs(time.Date(2026, 9, 24, 10, 0, 0, 0, time.UTC)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	factions, capitals, err := NewGenerator(db, 7).GenerateFactions()
	require.NoError(t, err)
	require.Equal(t, 1, factions)
	require.Equal(t, 1, capitals)
	require.NoError(t, mock.ExpectationsWereMet())
}
