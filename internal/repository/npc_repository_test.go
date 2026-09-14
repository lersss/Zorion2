// internal/repository/npc_repository_test.go
// Тесты репозитория NPC-агентов (спека 20a.1, этап 1): курсорные выборки
// (ListBatch / ListDueArrivals), batch-смены состояния (UpdateStatusBatch),
// Insert / Delete. Хелпер now() — общий с user_repository_test.go.
package repository

import (
	"database/sql"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"

	"zorion/internal/models"
)

// ==================== LIST ====================

// Курсорная выборка idle-агентов: статус, курсор и лимит в аргументах;
// after = "" кодируется нулевым uuid (курсор «с начала»). NULL-поля полёта
// сканируются в nil-указатели модели.
func TestNPCListBatchCursor(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectQuery(`SELECT id, name, status, current_world_id, from_world_id, target_world_id, depart_at, arrive_at, notify_enabled, last_observed_at, created_at, updated_at FROM npc_agents WHERE status = \$1 AND id > \$2 ORDER BY id LIMIT \$3`).
		WithArgs("idle", "00000000-0000-0000-0000-000000000000", 10).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "name", "status", "current_world_id", "from_world_id", "target_world_id", "depart_at", "arrive_at", "notify_enabled", "last_observed_at", "created_at", "updated_at",
		}).AddRow("a1", "Наблюдатель-1", "idle", "w1", nil, nil, nil, nil, true, nil, now(), now()))

	agents, err := NewNPCRepository(db).ListBatch(models.NPCAgentStatusIdle, "", 10)
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
	require.Len(t, agents, 1)
	require.Equal(t, "a1", agents[0].ID)
	require.Equal(t, models.NPCAgentStatusIdle, agents[0].Status)
	require.Equal(t, "w1", agents[0].CurrentWorldID)
	require.Nil(t, agents[0].FromWorldID, "idle-агент не в полёте — from/target/depart/arrive = NULL")
	require.Nil(t, agents[0].TargetWorldID)
	require.Nil(t, agents[0].DepartAt)
	require.Nil(t, agents[0].ArriveAt)
	require.Nil(t, agents[0].LastObservedAt)
	require.True(t, agents[0].NotifyEnabled)
}

// Прибытия: status='flying' и arrive_at <= now — фильтр по времени в SQL
// (приоритет прибытий бюджета тика, §2.2.A). Поля полёта сканируются.
func TestNPCListDueArrivals(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectQuery(`SELECT id, name, status, current_world_id, from_world_id, target_world_id, depart_at, arrive_at, notify_enabled, last_observed_at, created_at, updated_at FROM npc_agents WHERE status = 'flying' AND arrive_at <= \$1 AND id > \$2 ORDER BY id LIMIT \$3`).
		WithArgs(now(), "00000000-0000-0000-0000-000000000000", 5).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "name", "status", "current_world_id", "from_world_id", "target_world_id", "depart_at", "arrive_at", "notify_enabled", "last_observed_at", "created_at", "updated_at",
		}).AddRow("a2", "Наблюдатель-2", "flying", "w1", "w1", "w2", now().Add(-time.Minute), now().Add(-time.Second), true, nil, now(), now()))

	agents, err := NewNPCRepository(db).ListDueArrivals(now(), "", 5)
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
	require.Len(t, agents, 1)
	require.Equal(t, models.NPCAgentStatusFlying, agents[0].Status)
	require.NotNil(t, agents[0].FromWorldID)
	require.Equal(t, "w2", *agents[0].TargetWorldID)
	require.NotNil(t, agents[0].DepartAt)
	require.NotNil(t, agents[0].ArriveAt)
	require.Equal(t, "w1", agents[0].CurrentWorldID, "во время полёта current_world_id = старт")
}

// ==================== GET BY ID ====================

func TestNPCGetByID(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectQuery(`SELECT id, name, status, current_world_id, from_world_id, target_world_id, depart_at, arrive_at, notify_enabled, last_observed_at, created_at, updated_at FROM npc_agents WHERE id = \$1`).
		WithArgs("a1").
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "name", "status", "current_world_id", "from_world_id", "target_world_id", "depart_at", "arrive_at", "notify_enabled", "last_observed_at", "created_at", "updated_at",
		}).AddRow("a1", "Наблюдатель-1", "idle", "w1", nil, nil, nil, nil, true, nil, now(), now()))

	agent, err := NewNPCRepository(db).GetByID("a1")
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
	require.NotNil(t, agent)
	require.Equal(t, "a1", agent.ID)
	require.Nil(t, agent.TargetWorldID)
}

func TestNPCGetByIDNotFound(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectQuery(`SELECT id, name, status, current_world_id, from_world_id, target_world_id, depart_at, arrive_at, notify_enabled, last_observed_at, created_at, updated_at FROM npc_agents WHERE id = \$1`).
		WithArgs("nope").
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "name", "status", "current_world_id", "from_world_id", "target_world_id", "depart_at", "arrive_at", "notify_enabled", "last_observed_at", "created_at", "updated_at",
		}))

	agent, err := NewNPCRepository(db).GetByID("nope")
	require.NoError(t, err)
	require.Nil(t, agent)
}

// ==================== UPDATE (PATCH) ====================

// Частичное обновление: только имя — notify_enabled не трогается.
func TestNPCUpdateNameOnly(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectExec(`UPDATE npc_agents SET name = \$1, updated_at = NOW\(\) WHERE id = \$2`).
		WithArgs("Новое имя", "a1").
		WillReturnResult(sqlmock.NewResult(0, 1))

	name := "Новое имя"
	require.NoError(t, NewNPCRepository(db).Update("a1", &name, nil))
	require.NoError(t, mock.ExpectationsWereMet())
}

// Частичное обновление: только notify_enabled — имя не трогается.
func TestNPCUpdateNotifyOnly(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectExec(`UPDATE npc_agents SET notify_enabled = \$1, updated_at = NOW\(\) WHERE id = \$2`).
		WithArgs(false, "a1").
		WillReturnResult(sqlmock.NewResult(0, 1))

	enabled := false
	require.NoError(t, NewNPCRepository(db).Update("a1", nil, &enabled))
	require.NoError(t, mock.ExpectationsWereMet())
}

// Оба поля сразу — один UPDATE, порядок SET: name, notify_enabled.
func TestNPCUpdateBoth(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectExec(`UPDATE npc_agents SET name = \$1, notify_enabled = \$2, updated_at = NOW\(\) WHERE id = \$3`).
		WithArgs("Новое имя", true, "a1").
		WillReturnResult(sqlmock.NewResult(0, 1))

	name := "Новое имя"
	enabled := true
	require.NoError(t, NewNPCRepository(db).Update("a1", &name, &enabled))
	require.NoError(t, mock.ExpectationsWereMet())
}

// Агент не найден — sql.ErrNoRows.
func TestNPCUpdateNotFound(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectExec(`UPDATE npc_agents SET name = \$1, updated_at = NOW\(\) WHERE id = \$2`).
		WithArgs("Новое имя", "nope").
		WillReturnResult(sqlmock.NewResult(0, 0))

	name := "Новое имя"
	err = NewNPCRepository(db).Update("nope", &name, nil)
	require.ErrorIs(t, err, sql.ErrNoRows)
	require.NoError(t, mock.ExpectationsWereMet())
}

// Нет полей — ничего не обновляется, запроса нет.
func TestNPCUpdateNoFields(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
	require.NoError(t, err)
	defer db.Close()

	require.NoError(t, NewNPCRepository(db).Update("a1", nil, nil))
	require.NoError(t, mock.ExpectationsWereMet())
}

// ==================== LIST ALL ====================

// ListAll — все агенты без фильтров (для пересчёта позиций на тик).
func TestNPCListAll(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectQuery(`SELECT id, name, status, current_world_id, from_world_id, target_world_id, depart_at, arrive_at, notify_enabled, last_observed_at, created_at, updated_at FROM npc_agents`).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "name", "status", "current_world_id", "from_world_id", "target_world_id", "depart_at", "arrive_at", "notify_enabled", "last_observed_at", "created_at", "updated_at",
		}).AddRow("a1", "Наблюдатель-1", "flying", "w1", "w1", "w2", now(), now().Add(time.Minute), true, nil, now(), now()))

	agents, err := NewNPCRepository(db).ListAll()
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
	require.Len(t, agents, 1)
	require.Equal(t, models.NPCAgentStatusFlying, agents[0].Status)
	require.NotNil(t, agents[0].TargetWorldID)
}

// ==================== UPDATE STATUS (batch) ====================

// Прибытие (flying → idle): цель становится текущим миром, ставится
// last_observed_at — всё одной транзакцией.
func TestNPCUpdateStatusBatchArrival(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE npc_agents SET status = 'idle', current_world_id = \$1, last_observed_at = \$2, updated_at = NOW\(\) WHERE id = \$3`).
		WithArgs("w2", now(), "a1").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`UPDATE npc_agents SET status = 'idle', current_world_id = \$1, last_observed_at = \$2, updated_at = NOW\(\) WHERE id = \$3`).
		WithArgs("w5", now(), "a2").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	updates := []models.AgentStatusUpdate{
		{ID: "a1", Status: models.NPCAgentStatusIdle, CurrentWorldID: "w2", LastObservedAt: &[]time.Time{now()}[0]},
		{ID: "a2", Status: models.NPCAgentStatusIdle, CurrentWorldID: "w5", LastObservedAt: &[]time.Time{now()}[0]},
	}
	require.NoError(t, NewNPCRepository(db).UpdateStatusBatch(updates))
	require.NoError(t, mock.ExpectationsWereMet())
}

// Старт (idle → flying): заполняется кортеж полёта.
func TestNPCUpdateStatusBatchStart(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE npc_agents SET status = 'flying', from_world_id = \$1, target_world_id = \$2, depart_at = \$3, arrive_at = \$4, updated_at = NOW\(\) WHERE id = \$5`).
		WithArgs("w1", "w2", now(), now().Add(time.Minute), "a1").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	depart := now()
	arrive := now().Add(time.Minute)
	updates := []models.AgentStatusUpdate{
		{ID: "a1", Status: models.NPCAgentStatusFlying, FromWorldID: "w1", TargetWorldID: "w2", DepartAt: &depart, ArriveAt: &arrive},
	}
	require.NoError(t, NewNPCRepository(db).UpdateStatusBatch(updates))
	require.NoError(t, mock.ExpectationsWereMet())
}

// Смешанный batch: прибытия и старты в одном тике, порядок сохранён.
func TestNPCUpdateStatusBatchMixed(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE npc_agents SET status = 'idle', current_world_id = \$1, last_observed_at = \$2, updated_at = NOW\(\) WHERE id = \$3`).
		WithArgs("w2", now(), "a1").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`UPDATE npc_agents SET status = 'flying', from_world_id = \$1, target_world_id = \$2, depart_at = \$3, arrive_at = \$4, updated_at = NOW\(\) WHERE id = \$5`).
		WithArgs("w3", "w4", now(), now().Add(time.Minute), "a3").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	depart := now()
	arrive := now().Add(time.Minute)
	updates := []models.AgentStatusUpdate{
		{ID: "a1", Status: models.NPCAgentStatusIdle, CurrentWorldID: "w2", LastObservedAt: &[]time.Time{now()}[0]},
		{ID: "a3", Status: models.NPCAgentStatusFlying, FromWorldID: "w3", TargetWorldID: "w4", DepartAt: &depart, ArriveAt: &arrive},
	}
	require.NoError(t, NewNPCRepository(db).UpdateStatusBatch(updates))
	require.NoError(t, mock.ExpectationsWereMet())
}

// Пустой batch — ни одной операции с БД.
func TestNPCUpdateStatusBatchEmpty(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
	require.NoError(t, err)
	defer db.Close()

	require.NoError(t, NewNPCRepository(db).UpdateStatusBatch(nil))
	require.NoError(t, mock.ExpectationsWereMet())
}

// ==================== INSERT ====================

// Создание агента: пустой Status в модели → idle (спека §8: «ставит его
// в status='idle' на стартовый мир»).
func TestNPCInsertDefaultsIdle(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectExec(`INSERT INTO npc_agents \(id, name, status, current_world_id, from_world_id, target_world_id, depart_at, arrive_at, notify_enabled, last_observed_at, created_at, updated_at\) VALUES \(\$1, \$2, \$3, \$4, \$5, \$6, \$7, \$8, \$9, \$10, NOW\(\), NOW\(\)\)`).
		WithArgs("a1", "Наблюдатель-1", "idle", "w1", nil, nil, nil, nil, true, nil).
		WillReturnResult(sqlmock.NewResult(0, 1))

	agent := &models.NPCAgent{ID: "a1", Name: "Наблюдатель-1", CurrentWorldID: "w1"}
	require.NoError(t, NewNPCRepository(db).Insert(agent))
	require.NoError(t, mock.ExpectationsWereMet())
	require.Equal(t, models.NPCAgentStatusIdle, agent.Status, "модель должна получить статус idle")
	require.True(t, agent.NotifyEnabled, "создание всегда с уведомлениями (спека §8)")
}

// ==================== DELETE ====================

func TestNPCDelete(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectExec(`DELETE FROM npc_agents WHERE id = \$1`).
		WithArgs("a1").
		WillReturnResult(sqlmock.NewResult(0, 1))

	require.NoError(t, NewNPCRepository(db).Delete("a1"))
	require.NoError(t, mock.ExpectationsWereMet())
}

// Удаление несуществующего агента — sql.ErrNoRows (как у user_repository).
func TestNPCDeleteNotFound(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectExec(`DELETE FROM npc_agents WHERE id = \$1`).
		WithArgs("nope").
		WillReturnResult(sqlmock.NewResult(0, 0))

	err = NewNPCRepository(db).Delete("nope")
	require.ErrorIs(t, err, sql.ErrNoRows)
	require.NoError(t, mock.ExpectationsWereMet())
}