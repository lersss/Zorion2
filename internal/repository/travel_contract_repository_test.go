// internal/repository/travel_contract_repository_test.go
//
// B2b: NPC-агент как исполнитель контракта-перелёта (спека
// 2026-09-22-контракт-перелёт-и-доска §1.5). Тесты: выборка открытых перелётов
// с целью-системой (цель-планета отсекается), взятие агентом через общий
// атомарный flip, закрытие по прибытии с выпуском залога агенту, отсев чужой
// цели, пустой батч без обращений к БД.
package repository

import (
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"

	"zorion/internal/models"
)

// Выборка открытых перелётов для планировщика: только цель-система
// (dest_planet_id IS NULL) — контракт с целью-планетой агент не берёт (§1.5).
func TestListOpenSystemTravelsExcludesPlanetTarget(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectQuery(`(?s)SELECT id, payload->>'from_world_id', payload->>'dest_world_id' FROM contracts\s+` +
		`WHERE type = 'travel' AND status = 'open' AND expires_at > NOW\(\)\s+` +
		`AND payload->>'dest_planet_id' IS NULL`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "from_world_id", "dest_world_id"}).
			AddRow("c1", "w1", "w2"))

	refs, err := NewContractRepository(db).ListOpenSystemTravels()
	require.NoError(t, err)
	require.Equal(t, []models.TravelContractRef{
		{ID: "c1", FromWorldID: "w1", DestWorldID: "w2"},
	}, refs)
	require.NoError(t, mock.ExpectationsWereMet())
}

// Взятие агентом — тот же атомарный flip, что у игрока (Take), с
// executor_type='agent' и счётом агента; срок перебазируется от взятия
// (expires_at = $5, правило типа travel §4.3).
func TestTakeTravelAgent(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	rebase := time.Now().Add(45 * time.Second)
	mock.ExpectBegin()
	mock.ExpectExec(`INSERT INTO accounts`).
		WithArgs("agent", "a1", int64(0)).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(`UPDATE contracts\s+SET status = 'taken'`).
		WithArgs("c1", "agent", "a1", sqlmock.AnyArg(), rebase).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`INSERT INTO contract_log`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	ok, err := NewContractRepository(db).TakeTravel("c1", "a1", &rebase)
	require.NoError(t, err)
	require.True(t, ok)
	require.NoError(t, mock.ExpectationsWereMet())
}

// Гонка при взятии: 0 строк → false (контракт уже взят другим агентом).
func TestTakeTravelAgentConflict(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	rebase := time.Now().Add(45 * time.Second)
	mock.ExpectBegin()
	mock.ExpectExec(`INSERT INTO accounts`).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(`UPDATE contracts\s+SET status = 'taken'`).
		WithArgs("c1", "agent", "a1", sqlmock.AnyArg(), rebase).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectRollback()

	ok, err := NewContractRepository(db).TakeTravel("c1", "a1", &rebase)
	require.NoError(t, err)
	require.False(t, ok)
	require.NoError(t, mock.ExpectationsWereMet())
}

// Закрытие по прибытии: залог перелёта выпускается агенту-исполнителю
// (счёт owner_type='agent'), порядок истечение → завершение, одна транзакция.
func TestCloseArrivedTravelsReleasesEscrowToAgent(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectBegin()
	mock.ExpectQuery(`(?s)SELECT id, executor_id, payload->>'dest_world_id' FROM contracts\s+` +
		`WHERE type = 'travel' AND status = 'taken' AND executor_type = 'agent'`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "executor_id", "dest_world_id"}).
			AddRow("c1", "a1", "w2"))
	expectExpireDueStmt(mock, rowSet6()) // не истёк
	mock.ExpectQuery(`(?s)UPDATE contracts\s+SET status = 'completed'.*expires_at > NOW\(\)`).
		WithArgs("c1", "a1").
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "author_type", "author_id", "executor_type", "executor_id",
			"escrow_amount", "escrow_withdrawable", "funding",
		}).AddRow("c1", "faction", "f1", "agent", "a1", 500, 0, "regular"))
	mock.ExpectExec(`INSERT INTO accounts`).
		WithArgs("agent", "a1", int64(0)).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery(`UPDATE accounts\s+SET balance = balance \+ \$3`).
		WithArgs("agent", "a1", int64(500), int64(0)).
		WillReturnRows(sqlmock.NewRows([]string{"balance"}).AddRow(500))
	mock.ExpectExec(`INSERT INTO money_operations`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`INSERT INTO contract_log`).WillReturnResult(sqlmock.NewResult(0, 1)) // completed
	mock.ExpectExec(`INSERT INTO contract_log`).WillReturnResult(sqlmock.NewResult(0, 1)) // escrow_released
	mock.ExpectCommit()

	n, err := NewContractRepository(db).CloseArrivedTravels([]models.AgentTravelArrival{{AgentID: "a1", WorldID: "w2"}})
	require.NoError(t, err)
	require.Equal(t, 1, n)
	require.NoError(t, mock.ExpectationsWereMet())
}

// Перелёт агента с чужой целью-системой не закрывается: пара
// (executor_id, dest) должна совпасть с прибытием (agent a1 прибыл в w2,
// контракт ведёт в w9) → 0 завершённых.
func TestCloseArrivedTravelsIgnoresOtherDest(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectBegin()
	mock.ExpectQuery(`(?s)SELECT id, executor_id, payload->>'dest_world_id' FROM contracts`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "executor_id", "dest_world_id"}).
			AddRow("c1", "a1", "w9"))
	mock.ExpectCommit()

	n, err := NewContractRepository(db).CloseArrivedTravels([]models.AgentTravelArrival{{AgentID: "a1", WorldID: "w2"}})
	require.NoError(t, err)
	require.Equal(t, 0, n, "чужая цель не закрывает контракт")
	require.NoError(t, mock.ExpectationsWereMet())
}

// Пустой батч прибытий — без обращений к БД (цена не растёт на холостом тике).
func TestCloseArrivedTravelsEmpty(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	n, err := NewContractRepository(db).CloseArrivedTravels(nil)
	require.NoError(t, err)
	require.Equal(t, 0, n)
	require.NoError(t, mock.ExpectationsWereMet())
}
