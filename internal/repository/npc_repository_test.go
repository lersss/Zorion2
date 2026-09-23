// internal/repository/npc_repository_test.go
// Тесты репозитория NPC-агентов (спека 20a.1, этап 1): курсорные выборки
// (ListBatch / ListDueArrivals), batch-смены состояния (UpdateStatusBatch),
// Insert / Delete. Хелпер now() — общий с user_repository_test.go.
package repository

import (
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"

	"zorion/internal/models"
	"zorion/internal/npc"
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

// RaceHomeworlds — пул «раса → родной мир» (спека 2026-09-23 §5.1): мир
// берётся у родной планеты фракции (factions.homeworld_id → planets.world_id),
// т.к. homeworld_id ссылается на planets(id), а агент живёт в мире.
func TestNPCRaceHomeworlds(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectQuery(`SELECT f\.race_id, p\.world_id\s+FROM factions f\s+JOIN planets p ON p\.id = f\.homeworld_id\s+WHERE f\.race_id IS NOT NULL AND f\.homeworld_id IS NOT NULL`).
		WillReturnRows(sqlmock.NewRows([]string{"race_id", "world_id"}).
			AddRow("humans", "w1").
			AddRow("coastal", "w2"))

	out, err := NewNPCRepository(db).RaceHomeworlds()
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
	require.Len(t, out, 2)
	require.Equal(t, npc.RaceHomeworld{RaceID: "humans", HomeworldID: "w1"}, out[0])
	require.Equal(t, npc.RaceHomeworld{RaceID: "coastal", HomeworldID: "w2"}, out[1])
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

// Прибытия (flying → idle): цель становится текущим миром, ставится
// last_observed_at — ОДИН multi-row UPDATE через VALUES-джойн (идея 26c, A1:
// до 2000 одиночных Exec на тик → 1 запрос), всё одной транзакцией.
func TestNPCUpdateStatusBatchArrival(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE npc_agents SET status = 'idle'.*FROM \(VALUES.*AS v\(id, current_world_id, last_observed_at\) WHERE npc_agents\.id = v\.id`).
		WithArgs("a1", "w2", now(), "a2", "w5", now()).
		WillReturnResult(sqlmock.NewResult(0, 2))
	mock.ExpectCommit()

	updates := []models.AgentStatusUpdate{
		{ID: "a1", Status: models.NPCAgentStatusIdle, CurrentWorldID: "w2", LastObservedAt: &[]time.Time{now()}[0]},
		{ID: "a2", Status: models.NPCAgentStatusIdle, CurrentWorldID: "w5", LastObservedAt: &[]time.Time{now()}[0]},
	}
	require.NoError(t, NewNPCRepository(db).UpdateStatusBatch(updates))
	require.NoError(t, mock.ExpectationsWereMet())
}

// Старты (idle → flying): заполняется кортеж полёта — один multi-row UPDATE.
func TestNPCUpdateStatusBatchStart(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE npc_agents SET status = 'flying'.*FROM \(VALUES.*AS v\(id, from_world_id, target_world_id, depart_at, arrive_at\) WHERE npc_agents\.id = v\.id`).
		WithArgs("a1", "w1", "w2", now(), now().Add(time.Minute)).
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

// Смешанный batch: прибытия и старты в одном тике — два multi-row UPDATE
// (прибытия сначала, старты потом), одна транзакция.
func TestNPCUpdateStatusBatchMixed(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE npc_agents SET status = 'idle'.*FROM \(VALUES.*AS v\(id, current_world_id, last_observed_at\) WHERE npc_agents\.id = v\.id`).
		WithArgs("a1", "w2", now()).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`UPDATE npc_agents SET status = 'flying'.*FROM \(VALUES.*AS v\(id, from_world_id, target_world_id, depart_at, arrive_at\) WHERE npc_agents\.id = v\.id`).
		WithArgs("a3", "w3", "w4", now(), now().Add(time.Minute)).
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

// Большой batch (100 прибытий) — один multi-row UPDATE с 300 параметрами
// (порядок аргументов: id, current_world_id, last_observed_at на строку).
func TestNPCUpdateStatusBatchLarge(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	const n = 100
	updates := make([]models.AgentStatusUpdate, 0, n)
	expectedArgs := make([]driver.Value, 0, n*3)
	for i := 0; i < n; i++ {
		id := fmt.Sprintf("a%03d", i)
		world := fmt.Sprintf("w%03d", i)
		updates = append(updates, models.AgentStatusUpdate{
			ID: id, Status: models.NPCAgentStatusIdle,
			CurrentWorldID: world, LastObservedAt: &[]time.Time{now()}[0],
		})
		expectedArgs = append(expectedArgs, id, world, now())
	}

	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE npc_agents SET status = 'idle'.*FROM \(VALUES.*AS v\(id, current_world_id, last_observed_at\) WHERE npc_agents\.id = v\.id`).
		WithArgs(expectedArgs...).
		WillReturnResult(sqlmock.NewResult(0, n))
	mock.ExpectCommit()

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

// Неизвестный статус — ошибка без обращения к БД (как в старом applyAgentUpdate).
func TestNPCUpdateStatusBatchUnknownStatus(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
	require.NoError(t, err)
	defer db.Close()

	updates := []models.AgentStatusUpdate{
		{ID: "a1", Status: models.NPCAgentStatus("teleporting")},
	}
	err = NewNPCRepository(db).UpdateStatusBatch(updates)
	require.Error(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

// ==================== INSERT ====================

// Создание агента: пустой Status в модели → idle (спека §8: «ставит его
// в status='idle' на стартовый мир»). notify_enabled = false — дефолт новых
// агентов (спека 26a.1 §7.4: пуши — только через глобальный рубильник).
func TestNPCInsertDefaultsIdle(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectExec(`INSERT INTO npc_agents \(id, name, status, current_world_id, from_world_id, target_world_id, depart_at, arrive_at, notify_enabled, last_observed_at, created_at, updated_at\) VALUES \(\$1, \$2, \$3, \$4, \$5, \$6, \$7, \$8, \$9, \$10, NOW\(\), NOW\(\)\)`).
		WithArgs("a1", "Наблюдатель-1", "idle", "w1", nil, nil, nil, nil, false, nil).
		WillReturnResult(sqlmock.NewResult(0, 1))

	agent := &models.NPCAgent{ID: "a1", Name: "Наблюдатель-1", CurrentWorldID: "w1"}
	require.NoError(t, NewNPCRepository(db).Insert(agent))
	require.NoError(t, mock.ExpectationsWereMet())
	require.Equal(t, models.NPCAgentStatusIdle, agent.Status, "модель должна получить статус idle")
	require.False(t, agent.NotifyEnabled, "дефолт новых агентов — notify_enabled=false (спека 26a.1 §7.4)")
}

// ==================== BULK INSERT (COPY) ====================

// BulkInsert — пачка одной COPY-операцией (pq.CopyIn, спека 26a.1 §4.2):
// одна транзакция, статус idle, notify_enabled=false, race_id — раса агента
// (спека 2026-09-23 §5.2). Колонки — только создание; кортеж полёта не входит
// (DEFAULT NULL).
func TestNPCBulkInsert(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectBegin()
	mock.ExpectPrepare(`COPY "npc_agents" \("id", "name", "status", "current_world_id", "race_id", "notify_enabled", "created_at", "updated_at"\) FROM STDIN`)
	mock.ExpectExec(`COPY "npc_agents" \("id", "name", "status", "current_world_id", "race_id", "notify_enabled", "created_at", "updated_at"\) FROM STDIN`).
		WithArgs("a1", "Marion Hale", "idle", "w1", "humans", false, sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`COPY "npc_agents" \("id", "name", "status", "current_world_id", "race_id", "notify_enabled", "created_at", "updated_at"\) FROM STDIN`).
		WithArgs("a2", "Cyrus Venn", "idle", "w2", "coastal", false, sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`COPY "npc_agents" \("id", "name", "status", "current_world_id", "race_id", "notify_enabled", "created_at", "updated_at"\) FROM STDIN`).
		WillReturnResult(sqlmock.NewResult(0, 0)) // flush без аргументов
	mock.ExpectCommit()

	agents := []models.NPCAgent{
		{ID: "a1", Name: "Marion Hale", CurrentWorldID: "w1", RaceID: "humans", NotifyEnabled: true}, // принудительно true — COPY должен писать false
		{ID: "a2", Name: "Cyrus Venn", CurrentWorldID: "w2", RaceID: "coastal"},
	}
	require.NoError(t, NewNPCRepository(db).BulkInsert(agents))
	require.NoError(t, mock.ExpectationsWereMet())
}

// Ошибка в середине COPY → rollback: ни одна строка не вставлена.
func TestNPCBulkInsertRollback(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectBegin()
	mock.ExpectPrepare(`COPY "npc_agents" \("id", "name", "status", "current_world_id", "race_id", "notify_enabled", "created_at", "updated_at"\) FROM STDIN`)
	mock.ExpectExec(`COPY "npc_agents" \("id", "name", "status", "current_world_id", "race_id", "notify_enabled", "created_at", "updated_at"\) FROM STDIN`).
		WithArgs("a1", "Marion Hale", "idle", "w1", "humans", false, sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`COPY "npc_agents" \("id", "name", "status", "current_world_id", "race_id", "notify_enabled", "created_at", "updated_at"\) FROM STDIN`).
		WithArgs("a2", "Cyrus Venn", "idle", "w2", "coastal", false, sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnError(errors.New("boom"))
	mock.ExpectRollback()

	agents := []models.NPCAgent{
		{ID: "a1", Name: "Marion Hale", CurrentWorldID: "w1", RaceID: "humans"},
		{ID: "a2", Name: "Cyrus Venn", CurrentWorldID: "w2", RaceID: "coastal"},
	}
	err = NewNPCRepository(db).BulkInsert(agents)
	require.Error(t, err, "ошибка в середине COPY → ошибка транзакции")
	require.NoError(t, mock.ExpectationsWereMet())
}

// Пустой список — без обращения к БД.
func TestNPCBulkInsertEmpty(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
	require.NoError(t, err)
	defer db.Close()

	require.NoError(t, NewNPCRepository(db).BulkInsert(nil))
	require.NoError(t, mock.ExpectationsWereMet())
}

// ==================== LIST NAMES (seed уникальности) ====================

// ListNames — все имена агентов (спека 26a.1 §3.2: seed перед пачкой,
// уникальность между пачками). Одна колонка.
func TestNPCListNames(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectQuery(`SELECT name FROM npc_agents`).
		WillReturnRows(sqlmock.NewRows([]string{"name"}).
			AddRow("Marion Hale").AddRow("Cyrus Venn"))

	names, err := NewNPCRepository(db).ListNames()
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
	require.Equal(t, []string{"Marion Hale", "Cyrus Venn"}, names)
}

// ==================== LIST PAGE (keyset-пагинация) ====================

// Первая страница: без WHERE, ORDER BY created_at DESC, id DESC, LIMIT.
// next_cursor — opaque-строка точки последней записи (декодируется обратно).
func TestNPCListPageFirst(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	created := now()
	mock.ExpectQuery(`SELECT id, name, status, current_world_id, from_world_id, target_world_id, depart_at, arrive_at, notify_enabled, last_observed_at, created_at, updated_at FROM npc_agents ORDER BY created_at DESC, id DESC LIMIT \$1`).
		WithArgs(2).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "name", "status", "current_world_id", "from_world_id", "target_world_id", "depart_at", "arrive_at", "notify_enabled", "last_observed_at", "created_at", "updated_at",
		}).
			AddRow("a2", "Cyrus Venn", "idle", "w2", nil, nil, nil, nil, false, nil, created.Add(time.Minute), created.Add(time.Minute)).
			AddRow("a1", "Marion Hale", "idle", "w1", nil, nil, nil, nil, false, nil, created, created))

	agents, next, err := NewNPCRepository(db).ListPage(2, "")
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
	require.Len(t, agents, 2)
	require.Equal(t, "a2", agents[0].ID, "свежие сверху (created_at DESC)")
	require.NotEmpty(t, next, "страница полная — есть следующая")

	// Курсор — точка последней записи страницы: created_at + id.
	ct, id, err := decodeCursor(next)
	require.NoError(t, err)
	require.Equal(t, created, ct)
	require.Equal(t, "a1", id)
}

// Вторая страница: WHERE (created_at, id) < ($1, $2); последняя строка —
// next_cursor = "" (страниц больше нет).
func TestNPCListPageSecondAndEnd(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	created := now()
	cursor := encodeCursor(created, "a1")
	mock.ExpectQuery(`SELECT id, name, status, current_world_id, from_world_id, target_world_id, depart_at, arrive_at, notify_enabled, last_observed_at, created_at, updated_at FROM npc_agents WHERE \(created_at, id\) < \(\$1, \$2\) ORDER BY created_at DESC, id DESC LIMIT \$3`).
		WithArgs(created, "a1", 2).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "name", "status", "current_world_id", "from_world_id", "target_world_id", "depart_at", "arrive_at", "notify_enabled", "last_observed_at", "created_at", "updated_at",
		}).
			AddRow("a0", "Old One", "idle", "w0", nil, nil, nil, nil, false, nil, created.Add(-time.Minute), created.Add(-time.Minute)))

	agents, next, err := NewNPCRepository(db).ListPage(2, cursor)
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
	require.Len(t, agents, 1)
	require.Equal(t, "a0", agents[0].ID)
	require.Empty(t, next, "строк меньше лимита — конец списка")
}

// Невалидный cursor — ErrInvalidCursor (хендлер отдаёт 400).
func TestNPCListPageInvalidCursor(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
	require.NoError(t, err)
	defer db.Close()

	_, _, err = NewNPCRepository(db).ListPage(10, "!!!not-base64!!!")
	require.ErrorIs(t, err, ErrInvalidCursor)
	require.NoError(t, mock.ExpectationsWereMet())
}

// ==================== СЧЁТЧИКИ МЕТРИК ====================

func TestNPCCounts(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM npc_agents`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(12345))
	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM npc_agents WHERE status = \$1`).
		WithArgs("idle").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(3000))
	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM npc_agents WHERE status = \$1`).
		WithArgs("flying").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(9000))
	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM npc_agents WHERE status = 'flying' AND arrive_at <= \$1`).
		WithArgs(now()).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(500))

	repo := NewNPCRepository(db)
	total, err := repo.CountTotal()
	require.NoError(t, err)
	require.Equal(t, 12345, total)
	idle, err := repo.CountByStatus(models.NPCAgentStatusIdle)
	require.NoError(t, err)
	require.Equal(t, 3000, idle)
	flying, err := repo.CountByStatus(models.NPCAgentStatusFlying)
	require.NoError(t, err)
	require.Equal(t, 9000, flying)
	overdue, err := repo.CountOverdue(now())
	require.NoError(t, err)
	require.Equal(t, 500, overdue)
	require.NoError(t, mock.ExpectationsWereMet())
}

// ==================== ПОИСК ПО ИМЕНИ ====================

func TestNPCSearchByName(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectQuery(`SELECT id, name, status, current_world_id, target_world_id FROM npc_agents WHERE name ILIKE \$1 LIMIT \$2`).
		WithArgs("%hale%", 20).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "status", "current_world_id", "target_world_id"}).
			AddRow("a1", "Marion Hale", "flying", "w1", "w2").
			AddRow("a2", "Hale Bell", "idle", "w3", nil))

	agents, err := NewNPCRepository(db).SearchByName("hale", 20)
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
	require.Len(t, agents, 2)
	require.Equal(t, "a1", agents[0].ID)
	require.NotNil(t, agents[0].TargetWorldID, "летящий — цель полёта")
	require.Equal(t, "w2", *agents[0].TargetWorldID)
	require.Nil(t, agents[1].TargetWorldID, "idle — цели нет")
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

// DeleteAll — массовое удаление: один DELETE без WHERE (спека 26a.1, правка
// создателя 2026-09-15: атомарно и быстро, НЕ одиночные DELETE по id).
func TestNPCDeleteAll(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectExec(`DELETE FROM npc_agents`).
		WillReturnResult(sqlmock.NewResult(0, 7))

	deleted, err := NewNPCRepository(db).DeleteAll()
	require.NoError(t, err)
	require.Equal(t, int64(7), deleted, "число удалённых строк")
	require.NoError(t, mock.ExpectationsWereMet())
}

// Пустая таблица — 0 удалено, не ошибка.
func TestNPCDeleteAllEmpty(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectExec(`DELETE FROM npc_agents`).
		WillReturnResult(sqlmock.NewResult(0, 0))

	deleted, err := NewNPCRepository(db).DeleteAll()
	require.NoError(t, err)
	require.Equal(t, int64(0), deleted)
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