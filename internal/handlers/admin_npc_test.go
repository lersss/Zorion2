// internal/handlers/admin_npc_test.go
// Тесты ручек NPC-агентов (спека 20a.1 §8): список, создание, удаление,
// частичное обновление, настройки менеджера, позиции для карты.
package handlers

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"

	"zorion/internal/generator"
	"zorion/internal/mapcache"
	"zorion/internal/models"
	"zorion/internal/npc"
	"zorion/internal/repository"
)

// ==================== FAKE-ЗАВИСИМОСТИ МЕНЕДЖЕРА ====================

// npcFakeStore — пустое хранилище (для хендлеров БД заменяется sqlmock).
type npcFakeStore struct{}

func (npcFakeStore) ListDueArrivals(time.Time, string, int) ([]models.NPCAgent, error) { return nil, nil }
func (npcFakeStore) ListBatch(models.NPCAgentStatus, string, int) ([]models.NPCAgent, error) {
	return nil, nil
}
func (npcFakeStore) UpdateStatusBatch([]models.AgentStatusUpdate) error { return nil }
func (npcFakeStore) ListAll() ([]models.NPCAgent, error)                { return nil, nil }

// npcFakeWorlds — WorldSource без снапшота (RandomWorld → false).
type npcFakeWorlds struct{}

func (npcFakeWorlds) Snapshot() *mapcache.Snapshot { return nil }

// npcFakeRaces — RaceHomeworldSource в памяти (пул «раса → родной мир»,
// спека 2026-09-23 §5.1).
type npcFakeRaces struct{ origins []npc.RaceHomeworld }

func (f npcFakeRaces) RaceHomeworlds() ([]npc.RaceHomeworld, error) { return f.origins, nil }

// newAdminNPCHarness — sqlmock-БД + хендлеры NPC с менеджером без сетки.
func newAdminNPCHarness(t *testing.T) (*AdminNPCHandlers, sqlmock.Sqlmock) {
	t.Helper()
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	manager := npc.NewManager(npcFakeStore{}, npcFakeWorlds{}, nil, npc.DefaultSettings())
	return NewAdminNPCHandlers(
		repository.NewNPCRepository(db),
		repository.NewWorldRepository(db),
		manager,
	), mock
}

const agentID = "11111111-1111-1111-1111-111111111111"

// ==================== GET /admin/npc ====================

func TestAdminNPCListSuccess(t *testing.T) {
	h, mock := newAdminNPCHarness(t)

	mock.ExpectQuery(`SELECT id, name, status, current_world_id, race_id, from_world_id, target_world_id, depart_at, arrive_at, notify_enabled, last_observed_at, created_at, updated_at FROM npc_agents ORDER BY created_at DESC, id DESC LIMIT \$1`).
		WithArgs(200). // дефолтный limit (спека 26a.1 §5.2)
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "name", "status", "current_world_id", "race_id", "from_world_id", "target_world_id", "depart_at", "arrive_at", "notify_enabled", "last_observed_at", "created_at", "updated_at",
		}).AddRow(agentID, "Наблюдатель-1", "flying", "w1", "humans", "w1", "w2", now(), now().Add(time.Minute), true, nil, now(), now()))

	req := httptest.NewRequest(http.MethodGet, "/admin/npc", nil)
	rec := execJSON(h.HandleCollection, req)

	require.Equal(t, http.StatusOK, rec.Code)
	var resp struct {
		Agents []struct {
			ID     string `json:"id"`
			Name   string `json:"name"`
			Status string `json:"status"`
		} `json:"agents"`
		NextCursor interface{} `json:"next_cursor"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Len(t, resp.Agents, 1)
	require.Equal(t, "Наблюдатель-1", resp.Agents[0].Name)
	require.Equal(t, "flying", resp.Agents[0].Status)
	require.Nil(t, resp.NextCursor, "строк меньше limit — страниц больше нет (спека §5.2)")
	require.NoError(t, mock.ExpectationsWereMet())
}

// Пагинация: вторая страница по cursor, next_cursor при полной странице.
func TestAdminNPCListPagination(t *testing.T) {
	h, mock := newAdminNPCHarness(t)

	created := now()
	// Первая страница: ровно limit строк → next_cursor не null.
	mock.ExpectQuery(`SELECT id, name, status, current_world_id, race_id, from_world_id, target_world_id, depart_at, arrive_at, notify_enabled, last_observed_at, created_at, updated_at FROM npc_agents ORDER BY created_at DESC, id DESC LIMIT \$1`).
		WithArgs(1).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "name", "status", "current_world_id", "race_id", "from_world_id", "target_world_id", "depart_at", "arrive_at", "notify_enabled", "last_observed_at", "created_at", "updated_at",
		}).AddRow("a2", "Cyrus Venn", "idle", "w2", nil, nil, nil, nil, nil, false, nil, created.Add(time.Minute), created.Add(time.Minute)))

	req := httptest.NewRequest(http.MethodGet, "/admin/npc?limit=1", nil)
	rec := execJSON(h.HandleCollection, req)
	require.Equal(t, http.StatusOK, rec.Code)
	var first struct {
		Agents     []struct{ ID string `json:"id"` } `json:"agents"`
		NextCursor *string                            `json:"next_cursor"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &first))
	require.Len(t, first.Agents, 1)
	require.NotNil(t, first.NextCursor, "полная страница — есть следующая")
	cursor := *first.NextCursor

	// Вторая страница: WHERE (created_at, id) < ($1, $2); строк меньше
	// лимита — конец списка (next_cursor = null).
	mock.ExpectQuery(`SELECT id, name, status, current_world_id, race_id, from_world_id, target_world_id, depart_at, arrive_at, notify_enabled, last_observed_at, created_at, updated_at FROM npc_agents WHERE \(created_at, id\) < \(\$1, \$2\) ORDER BY created_at DESC, id DESC LIMIT \$3`).
		WithArgs(created.Add(time.Minute), "a2", 2).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "name", "status", "current_world_id", "race_id", "from_world_id", "target_world_id", "depart_at", "arrive_at", "notify_enabled", "last_observed_at", "created_at", "updated_at",
		}).AddRow("a1", "Marion Hale", "idle", "w1", nil, nil, nil, nil, nil, false, nil, created, created))

	req2 := httptest.NewRequest(http.MethodGet, "/admin/npc?limit=2&cursor="+cursor, nil)
	rec2 := execJSON(h.HandleCollection, req2)
	require.Equal(t, http.StatusOK, rec2.Code)
	var second struct {
		Agents     []struct{ ID string `json:"id"` } `json:"agents"`
		NextCursor *string                            `json:"next_cursor"`
	}
	require.NoError(t, json.Unmarshal(rec2.Body.Bytes(), &second))
	require.Len(t, second.Agents, 1)
	require.Nil(t, second.NextCursor, "строк меньше limit — конец списка")
	require.NoError(t, mock.ExpectationsWereMet())
}

// limit вне диапазона (0 / >1000) — 400 (спека §5.2).
func TestAdminNPCListInvalidLimit(t *testing.T) {
	h, _ := newAdminNPCHarness(t)

	for _, q := range []string{"limit=0", "limit=-1", "limit=abc", "limit=1001"} {
		req := httptest.NewRequest(http.MethodGet, "/admin/npc?"+q, nil)
		rec := execJSON(h.HandleCollection, req)
		require.Equal(t, http.StatusBadRequest, rec.Code, "limit=%s", q)
	}
}

// Невалидный cursor — 400 (спека §5.2).
func TestAdminNPCListInvalidCursor(t *testing.T) {
	h, _ := newAdminNPCHarness(t)

	req := httptest.NewRequest(http.MethodGet, "/admin/npc?cursor=!!!not-base64!!!", nil)
	rec := execJSON(h.HandleCollection, req)
	require.Equal(t, http.StatusBadRequest, rec.Code)
}

// ==================== POST /admin/npc/generate (спека 26a.1 §4) ====================

// npcGridWorlds — WorldSource с реальным снапшотом (загружен через
// mapcache.LoadAndSwap) — для тестов массовой генерации (сетка строится
// из снапшота, спека 26a.1 §4.4).
type npcGridWorlds struct{ m *mapcache.Manager }

func (g npcGridWorlds) Snapshot() *mapcache.Snapshot { return g.m.Snapshot() }

// newBulkHarness — sqlmock-БД + хендлеры NPC с менеджером, у которого сетка
// из двух миров (снапшот загружается через mapcache.LoadAndSwap) и пул
// «раса → родной мир» из двух рас. Каждая раса — в своём мире (спека §5.2).
func newBulkHarness(t *testing.T) (*AdminNPCHandlers, *sql.DB, sqlmock.Sqlmock) {
	t.Helper()
	return newBulkHarnessWithRaces(t, []npc.RaceHomeworld{
		{RaceID: "humans", HomeworldID: "w1"},
		{RaceID: "coastal", HomeworldID: "w2"},
	})
}

// newBulkHarnessWithRaces — тот же харнесс с заданным пулом расы → родной мир
// (origins=nil — пустой пул, спека §5.4).
func newBulkHarnessWithRaces(t *testing.T, origins []npc.RaceHomeworld) (*AdminNPCHandlers, *sql.DB, sqlmock.Sqlmock) {
	t.Helper()
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })

	mapCache := mapcache.NewManager()
	mock.ExpectQuery(`SELECT id, name, coord_x, coord_y, COALESCE\(spectral_class,''\), temperature, star_type, system_type, stellar_mods FROM worlds`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "coord_x", "coord_y", "spectral_class", "temperature", "star_type", "system_type", "stellar_mods"}).
			AddRow("w1", "Alpha", 1.0, 2.0, "G", 5600, "star", "single", nil).
			AddRow("w2", "Beta", 10.0, 20.0, "O", 42000, "star", "single", nil))
	mock.ExpectQuery(`SELECT p.world_id, p.data->>'life', p.data->>'type', p.data->'resources'`).
		WillReturnRows(sqlmock.NewRows([]string{"world_id", "life", "type", "resources", "settled"}))
	require.NoError(t, mapCache.LoadAndSwap(context.Background(), db))

	manager := npc.NewManager(npcFakeStore{}, npcGridWorlds{mapCache}, npcFakeRaces{origins}, npc.DefaultSettings())
	return NewAdminNPCHandlers(
		repository.NewNPCRepository(db),
		repository.NewWorldRepository(db),
		manager,
	), db, mock
}

// ==================== POST /admin/npc ====================

func TestAdminNPCCreateWithStartWorld(t *testing.T) {
	// Пул расы непуст (snapshot карты + источник): одиночное создание ставит
	// расу из пула (спека 2026-09-23 §5.2), стартовый мир — явно указанный.
	h, _, mock := newBulkHarnessWithRaces(t, []npc.RaceHomeworld{{RaceID: "humans", HomeworldID: "w1"}})

	mock.ExpectQuery(`SELECT id, name, coord_x, coord_y, COALESCE\(spectral_class,''\), temperature, star_type, system_type, stellar_mods, stellar_mass, age, created_at, updated_at FROM worlds WHERE id = \$1`).
		WithArgs("22222222-2222-2222-2222-222222222222").
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "name", "coord_x", "coord_y", "spectral_class", "temperature", "star_type", "system_type", "stellar_mods", "stellar_mass", "age", "created_at", "updated_at",
		}).AddRow("22222222-2222-2222-2222-222222222222", "Sirius", 0, 0, "A", 10000, "star", "single", nil, nil, nil, now(), now()))
	mock.ExpectExec(`INSERT INTO npc_agents \(id, name, status, current_world_id, race_id, from_world_id, target_world_id, depart_at, arrive_at, notify_enabled, last_observed_at, created_at, updated_at\) VALUES \(\$1, \$2, \$3, \$4, \$5, \$6, \$7, \$8, \$9, \$10, \$11, NOW\(\), NOW\(\)\)`).
		WithArgs(sqlmock.AnyArg(), "Наблюдатель-1", "idle", "22222222-2222-2222-2222-222222222222", "humans", nil, nil, nil, nil, false, nil).
		WillReturnResult(sqlmock.NewResult(0, 1))

	body := `{"name":"Наблюдатель-1","start_world_id":"22222222-2222-2222-2222-222222222222"}`
	req := httptest.NewRequest(http.MethodPost, "/admin/npc", strings.NewReader(body))
	rec := execJSON(h.HandleCollection, req)

	require.Equal(t, http.StatusCreated, rec.Code)
	var resp struct {
		ID     string `json:"id"`
		Status string `json:"status"`
		RaceID string `json:"race_id"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Equal(t, "idle", resp.Status, "создание ставит агента в idle (спека §8)")
	require.Equal(t, "humans", resp.RaceID, "одиночное создание ставит расу из пула (спека §5.2)")
	require.True(t, h.manager.IsAgentsDirty(), "одиночное создание инвалидирует кэш агентов (идея 26c A2)")
	require.NoError(t, mock.ExpectationsWereMet())
}

// Пустой пул «раса → родной мир» (фракции не сгенерированы) — 400 с понятным
// сообщением: новых агентов с race_id NULL не появляется (спека §5.2, §5.4).
func TestAdminNPCCreateNoRacePool(t *testing.T) {
	h, _, _ := newBulkHarnessWithRaces(t, nil)

	req := httptest.NewRequest(http.MethodPost, "/admin/npc", strings.NewReader(`{"name":"A"}`))
	rec := execJSON(h.HandleCollection, req)
	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Contains(t, rec.Body.String(), "Нет рас с родным миром")
}

func TestAdminNPCCreateEmptyName(t *testing.T) {
	h, _ := newAdminNPCHarness(t)

	req := httptest.NewRequest(http.MethodPost, "/admin/npc", strings.NewReader(`{"name":""}`))
	rec := execJSON(h.HandleCollection, req)
	require.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestAdminNPCCreateInvalidWorldID(t *testing.T) {
	h, _ := newAdminNPCHarness(t)

	req := httptest.NewRequest(http.MethodPost, "/admin/npc", strings.NewReader(`{"name":"A","start_world_id":"not-a-uuid"}`))
	rec := execJSON(h.HandleCollection, req)
	require.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestAdminNPCCreateWorldNotFound(t *testing.T) {
	h, mock := newAdminNPCHarness(t)

	mock.ExpectQuery(`SELECT id, name, coord_x, coord_y, COALESCE\(spectral_class,''\), temperature, star_type, system_type, stellar_mods, stellar_mass, age, created_at, updated_at FROM worlds WHERE id = \$1`).
		WithArgs("22222222-2222-2222-2222-222222222222").
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "name", "coord_x", "coord_y", "spectral_class", "temperature", "star_type", "system_type", "stellar_mods", "stellar_mass", "age", "created_at", "updated_at",
		}))

	body := `{"name":"A","start_world_id":"22222222-2222-2222-2222-222222222222"}`
	req := httptest.NewRequest(http.MethodPost, "/admin/npc", strings.NewReader(body))
	rec := execJSON(h.HandleCollection, req)
	require.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestAdminNPCCreateNoWorlds(t *testing.T) {
	// Источник пула рас не подключён (менеджер без фракций) — пул пуст → 400
	// «Нет рас с родным миром» (спека 2026-09-23 §5.4).
	h, _ := newAdminNPCHarness(t)

	req := httptest.NewRequest(http.MethodPost, "/admin/npc", strings.NewReader(`{"name":"A"}`))
	rec := execJSON(h.HandleCollection, req)
	require.Equal(t, http.StatusBadRequest, rec.Code)
}

// ==================== DELETE /admin/npc/{id} ====================

func TestAdminNPCDeleteSuccess(t *testing.T) {
	h, mock := newAdminNPCHarness(t)

	mock.ExpectExec(`DELETE FROM npc_agents WHERE id = \$1`).
		WithArgs(agentID).
		WillReturnResult(sqlmock.NewResult(0, 1))

	req := httptest.NewRequest(http.MethodDelete, "/admin/npc/"+agentID, nil)
	rec := execJSON(h.HandleObject, req)
	require.Equal(t, http.StatusNoContent, rec.Code)
	require.True(t, h.manager.IsAgentsDirty(), "одиночное удаление инвалидирует кэш агентов (идея 26c A2)")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestAdminNPCDeleteNotFound(t *testing.T) {
	h, mock := newAdminNPCHarness(t)

	mock.ExpectExec(`DELETE FROM npc_agents WHERE id = \$1`).
		WithArgs(agentID).
		WillReturnResult(sqlmock.NewResult(0, 0))

	req := httptest.NewRequest(http.MethodDelete, "/admin/npc/"+agentID, nil)
	rec := execJSON(h.HandleObject, req)
	require.Equal(t, http.StatusNotFound, rec.Code)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestAdminNPCInvalidID(t *testing.T) {
	h, _ := newAdminNPCHarness(t)

	req := httptest.NewRequest(http.MethodDelete, "/admin/npc/not-a-uuid", nil)
	rec := execJSON(h.HandleObject, req)
	require.Equal(t, http.StatusBadRequest, rec.Code)
}

// ==================== PATCH /admin/npc/{id} ====================

func TestAdminNPCPatchSuccess(t *testing.T) {
	h, mock := newAdminNPCHarness(t)

	mock.ExpectExec(`UPDATE npc_agents SET name = \$1, notify_enabled = \$2, updated_at = NOW\(\) WHERE id = \$3`).
		WithArgs("Новое имя", false, agentID).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(`SELECT id, name, status, current_world_id, race_id, from_world_id, target_world_id, depart_at, arrive_at, notify_enabled, last_observed_at, created_at, updated_at FROM npc_agents WHERE id = \$1`).
		WithArgs(agentID).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "name", "status", "current_world_id", "race_id", "from_world_id", "target_world_id", "depart_at", "arrive_at", "notify_enabled", "last_observed_at", "created_at", "updated_at",
		}).AddRow(agentID, "Новое имя", "idle", "w1", nil, nil, nil, nil, nil, false, nil, now(), now()))

	req := httptest.NewRequest(http.MethodPatch, "/admin/npc/"+agentID, strings.NewReader(`{"name":"Новое имя","notify_enabled":false}`))
	rec := execJSON(h.HandleObject, req)

	require.Equal(t, http.StatusOK, rec.Code)
	var resp struct {
		Name          string `json:"name"`
		NotifyEnabled bool   `json:"notify_enabled"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Equal(t, "Новое имя", resp.Name)
	require.False(t, resp.NotifyEnabled)
	require.True(t, h.manager.IsAgentsDirty(), "PATCH (имя/пуши) инвалидирует кэш агентов — имя видно на карте (идея 26c A2)")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestAdminNPCPatchNotFound(t *testing.T) {
	h, mock := newAdminNPCHarness(t)

	mock.ExpectExec(`UPDATE npc_agents SET name = \$1, updated_at = NOW\(\) WHERE id = \$2`).
		WithArgs("Новое имя", agentID).
		WillReturnResult(sqlmock.NewResult(0, 0))

	req := httptest.NewRequest(http.MethodPatch, "/admin/npc/"+agentID, strings.NewReader(`{"name":"Новое имя"}`))
	rec := execJSON(h.HandleObject, req)
	require.Equal(t, http.StatusNotFound, rec.Code)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestAdminNPCPatchNoFields(t *testing.T) {
	h, _ := newAdminNPCHarness(t)

	req := httptest.NewRequest(http.MethodPatch, "/admin/npc/"+agentID, strings.NewReader(`{}`))
	rec := execJSON(h.HandleObject, req)
	require.Equal(t, http.StatusBadRequest, rec.Code)
}

// ==================== POST /admin/npc/generate (спека 26a.1 §4) ====================

// count < 1, не-целое, пустое тело — 400 (лимита сверху нет: спека §4.3).
func TestAdminNPCGenerateInvalidCount(t *testing.T) {
	h, _, _ := newBulkHarness(t)

	for _, body := range []string{
		`{"count":0}`, `{"count":-1}`, `{"count":"abc"}`, `{}`, `{"count":1.5}`,
	} {
		req := httptest.NewRequest(http.MethodPost, "/admin/npc/generate", strings.NewReader(body))
		rec := execJSON(h.HandleObject, req)
		require.Equal(t, http.StatusBadRequest, rec.Code, "body=%s", body)
	}
}

// Нет пула «раса → родной мир» (снапшот карты не готов, источник не
// подключён) — 400 «Нет рас с родным миром» (спека 2026-09-23 §5.4).
func TestAdminNPCGenerateNoWorlds(t *testing.T) {
	h, _ := newAdminNPCHarness(t)

	req := httptest.NewRequest(http.MethodPost, "/admin/npc/generate", strings.NewReader(`{"count":5}`))
	rec := execJSON(h.HandleObject, req)
	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Contains(t, rec.Body.String(), "Нет рас с родным миром")
}

// Пул расы пуст (фракции не сгенерированы) при готовой карте — 400 с понятным
// сообщением, а не 500 и не пустая пачка (спека §5.4).
func TestAdminNPCGenerateEmptyRacePool(t *testing.T) {
	h, _, _ := newBulkHarnessWithRaces(t, nil)

	req := httptest.NewRequest(http.MethodPost, "/admin/npc/generate", strings.NewReader(`{"count":5}`))
	rec := execJSON(h.HandleObject, req)
	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Contains(t, rec.Body.String(), "Нет рас с родным миром")
}

// 409 — джоб generate_npc уже крутится (TryStart, защита от двойного клика).
func TestAdminNPCGenerateConflict(t *testing.T) {
	h, _, _ := newBulkHarness(t)

	_, cancel := context.WithCancel(context.Background())
	require.True(t, statusManager.TryStart(generator.JobGenerateNPC, 5, cancel))
	defer statusManager.Cancel(generator.JobGenerateNPC)

	req := httptest.NewRequest(http.MethodPost, "/admin/npc/generate", strings.NewReader(`{"count":5}`))
	rec := execJSON(h.HandleObject, req)
	require.Equal(t, http.StatusConflict, rec.Code)
}

// Успех: 202 → джоб доходит до done с отчётом «Создано агентов: N за X.X с»;
// BulkInsert — одной COPY-транзакцией (спека §4.1, §4.2). Агент получает расу
// и стартует в её родном мире (спека 2026-09-23 §5.2): один origin в пуле —
// проверяем точные пару (race_id, current_world_id) в COPY-строке.
func TestAdminNPCGenerateSuccess(t *testing.T) {
	h, _, mock := newBulkHarnessWithRaces(t, []npc.RaceHomeworld{{RaceID: "coastal", HomeworldID: "w2"}})

	// Джоб: seed имён (ListNames) + COPY-вставка одной транзакцией.
	mock.ExpectQuery(`SELECT name FROM npc_agents`).
		WillReturnRows(sqlmock.NewRows([]string{"name"}).AddRow("Старый Агент"))
	mock.ExpectBegin()
	mock.ExpectPrepare(`COPY "npc_agents"`)
	mock.ExpectExec(`COPY "npc_agents" \("id", "name", "status", "current_world_id", "race_id", "notify_enabled", "created_at", "updated_at"\) FROM STDIN`).
		WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), "idle", "w2", "coastal", false, sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`COPY "npc_agents" \("id", "name", "status", "current_world_id", "race_id", "notify_enabled", "created_at", "updated_at"\) FROM STDIN`).
		WillReturnResult(sqlmock.NewResult(0, 0)) // flush
	mock.ExpectCommit()

	req := httptest.NewRequest(http.MethodPost, "/admin/npc/generate", strings.NewReader(`{"count":1}`))
	rec := execJSON(h.HandleObject, req)
	require.Equal(t, http.StatusAccepted, rec.Code)

	// Ждём завершения джоба (паттерн admin_settlements_test).
	deadline := time.Now().Add(2 * time.Second)
	status := ""
	for time.Now().Before(deadline) {
		_, _, status, _, _ = statusManager.GetStatus(generator.JobGenerateNPC)
		if status != "running" {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	require.Equal(t, "done", status, "джоб должен завершиться успешно")
	_, _, _, _, report := statusManager.GetStatus(generator.JobGenerateNPC)
	require.Contains(t, report, "Создано агентов: 1")
	require.True(t, h.manager.IsAgentsDirty(), "массовая генерация инвалидирует кэш агентов (идея 26c A2)")
	require.NoError(t, mock.ExpectationsWereMet())
}

// ==================== GET /admin/npc/metrics (спека 26a.1 §8) ====================

func TestAdminNPCMetrics(t *testing.T) {
	h, mock := newAdminNPCHarness(t)

	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM npc_agents`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(12345))
	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM npc_agents WHERE status = \$1`).
		WithArgs("idle").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(3000))
	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM npc_agents WHERE status = \$1`).
		WithArgs("flying").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(9000))
	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM npc_agents WHERE status = 'flying' AND arrive_at <= \$1`).
		WithArgs(sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(500))

	req := httptest.NewRequest(http.MethodGet, "/admin/npc/metrics", nil)
	rec := execJSON(h.HandleObject, req)
	require.Equal(t, http.StatusOK, rec.Code)

	var resp struct {
		AgentsTotal     int `json:"agents_total"`
		AgentsIdle      int `json:"agents_idle"`
		AgentsFlying    int `json:"agents_flying"`
		OverdueArrivals int `json:"overdue_arrivals"`
		Scheduler       struct {
			TicksTotal  int64 `json:"ticks_total"`
			FullBatches int64 `json:"full_batches"`
		} `json:"scheduler"`
		LastBulk interface{} `json:"last_bulk"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Equal(t, 12345, resp.AgentsTotal)
	require.Equal(t, 3000, resp.AgentsIdle)
	require.Equal(t, 9000, resp.AgentsFlying)
	require.Equal(t, 500, resp.OverdueArrivals, "живой COUNT очереди на тик (вариант «а», спека §8.1)")
	require.Nil(t, resp.LastBulk, "пачки не было — last_bulk null")
	require.NoError(t, mock.ExpectationsWereMet())
}

// ==================== GET /api/npc/search (спека 26a.1 §6.1) ====================

// По имени — ILIKE; агента нет в PositionCache — x/y = null (клиент
// центрирует только при наличии, спека §6.2).
func TestAdminNPCSearchByName(t *testing.T) {
	h, mock := newAdminNPCHarness(t)

	mock.ExpectQuery(`SELECT id, name, status, current_world_id, target_world_id FROM npc_agents WHERE name ILIKE \$1 LIMIT \$2`).
		WithArgs("%hale%", 20).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "status", "current_world_id", "target_world_id"}).
			AddRow("a1", "Marion Hale", "flying", "w1", "w2"))

	req := httptest.NewRequest(http.MethodGet, "/api/npc/search?q=hale", nil)
	rec := execJSON(h.SearchAgent, req)
	require.Equal(t, http.StatusOK, rec.Code)

	var resp struct {
		Results []struct {
			ID      string  `json:"id"`
			Name    string  `json:"name"`
			Status  string  `json:"status"`
			X       *float64 `json:"x"`
			Y       *float64 `json:"y"`
			WorldID string  `json:"world_id"`
		} `json:"results"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Len(t, resp.Results, 1)
	require.Equal(t, "a1", resp.Results[0].ID)
	require.Equal(t, "w2", resp.Results[0].WorldID, "летящий — world_id = цель полёта")
	require.Nil(t, resp.Results[0].X, "агента нет в snapshot — x/y null")
	require.Nil(t, resp.Results[0].Y)
	require.NoError(t, mock.ExpectationsWereMet())
}

// Валидный UUID в q — точное совпадение по id (PK, 0–1 результат, спека §6.1).
func TestAdminNPCSearchByUUID(t *testing.T) {
	h, mock := newAdminNPCHarness(t)

	mock.ExpectQuery(`SELECT id, name, status, current_world_id, race_id, from_world_id, target_world_id, depart_at, arrive_at, notify_enabled, last_observed_at, created_at, updated_at FROM npc_agents WHERE id = \$1`).
		WithArgs(agentID).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "name", "status", "current_world_id", "race_id", "from_world_id", "target_world_id", "depart_at", "arrive_at", "notify_enabled", "last_observed_at", "created_at", "updated_at",
		}).AddRow(agentID, "Marion Hale", "idle", "w1", nil, nil, nil, nil, nil, false, nil, now(), now()))

	req := httptest.NewRequest(http.MethodGet, "/api/npc/search?q="+agentID, nil)
	rec := execJSON(h.SearchAgent, req)
	require.Equal(t, http.StatusOK, rec.Code)

	var resp struct {
		Results []struct {
			ID      string `json:"id"`
			WorldID string `json:"world_id"`
		} `json:"results"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Len(t, resp.Results, 1)
	require.Equal(t, agentID, resp.Results[0].ID)
	require.Equal(t, "w1", resp.Results[0].WorldID, "idle — world_id = текущий мир")
	require.NoError(t, mock.ExpectationsWereMet())
}

// UUID не найден — пустой results (не ошибка).
func TestAdminNPCSearchByUUIDNotFound(t *testing.T) {
	h, mock := newAdminNPCHarness(t)

	mock.ExpectQuery(`SELECT id, name, status, current_world_id, race_id, from_world_id, target_world_id, depart_at, arrive_at, notify_enabled, last_observed_at, created_at, updated_at FROM npc_agents WHERE id = \$1`).
		WithArgs(agentID).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "name", "status", "current_world_id", "race_id", "from_world_id", "target_world_id", "depart_at", "arrive_at", "notify_enabled", "last_observed_at", "created_at", "updated_at",
		}))

	req := httptest.NewRequest(http.MethodGet, "/api/npc/search?q="+agentID, nil)
	rec := execJSON(h.SearchAgent, req)
	require.Equal(t, http.StatusOK, rec.Code)
	var resp struct {
		Results []interface{} `json:"results"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Empty(t, resp.Results)
	require.NoError(t, mock.ExpectationsWereMet())
}

// q пуст/слишком длинный — 400 (спека §6.1).
func TestAdminNPCSearchInvalidQ(t *testing.T) {
	h, _ := newAdminNPCHarness(t)

	long := strings.Repeat("a", 101)
	for _, q := range []string{"q=", "q=%20%20%20", "q=" + long} {
		req := httptest.NewRequest(http.MethodGet, "/api/npc/search?"+q, nil)
		rec := execJSON(h.SearchAgent, req)
		require.Equal(t, http.StatusBadRequest, rec.Code, "q=%q", q)
	}
}

// ==================== POST /admin/npc/clear (правка 2026-09-15) ====================

// Успех: один DELETE без WHERE → {"deleted": N}.
func TestAdminNPCClearAllSuccess(t *testing.T) {
	h, mock := newAdminNPCHarness(t)

	mock.ExpectExec(`DELETE FROM npc_agents`).
		WillReturnResult(sqlmock.NewResult(0, 7))

	req := httptest.NewRequest(http.MethodPost, "/admin/npc/clear", nil)
	rec := execJSON(h.HandleObject, req)

	require.Equal(t, http.StatusOK, rec.Code)
	var resp struct {
		Deleted int64 `json:"deleted"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Equal(t, int64(7), resp.Deleted)
	require.True(t, h.manager.IsAgentsDirty(), "массовое удаление инвалидирует кэш агентов (идея 26c A2)")
	require.NoError(t, mock.ExpectationsWereMet())
}

// 409 — идёт джоб генерации (паттерн ClearUniverse: живые прогоны, пишущие
// в одни таблицы, не пересекаются — AGENTS.md §23).
func TestAdminNPCClearAllConflict(t *testing.T) {
	h, _ := newAdminNPCHarness(t)

	_, cancel := context.WithCancel(context.Background())
	require.True(t, statusManager.TryStart(generator.JobGenerateNPC, 5, cancel))
	defer statusManager.Cancel(generator.JobGenerateNPC)

	req := httptest.NewRequest(http.MethodPost, "/admin/npc/clear", nil)
	rec := execJSON(h.HandleObject, req)
	require.Equal(t, http.StatusConflict, rec.Code)
}

// Ошибка БД — 500.
func TestAdminNPCClearAllDBError(t *testing.T) {
	h, mock := newAdminNPCHarness(t)

	mock.ExpectExec(`DELETE FROM npc_agents`).
		WillReturnError(errors.New("boom"))

	req := httptest.NewRequest(http.MethodPost, "/admin/npc/clear", nil)
	rec := execJSON(h.HandleObject, req)
	require.Equal(t, http.StatusInternalServerError, rec.Code)
	require.NoError(t, mock.ExpectationsWereMet())
}

// Не-POST — 405.
func TestAdminNPCClearAllMethodNotAllowed(t *testing.T) {
	h, _ := newAdminNPCHarness(t)

	req := httptest.NewRequest(http.MethodGet, "/admin/npc/clear", nil)
	rec := execJSON(h.HandleObject, req)
	require.Equal(t, http.StatusMethodNotAllowed, rec.Code)
}

// ==================== НАСТРОЙКИ /admin/npc/settings ====================

func TestAdminNPCSettingsGet(t *testing.T) {
	h, _ := newAdminNPCHarness(t)

	req := httptest.NewRequest(http.MethodGet, "/admin/npc/settings", nil)
	rec := execJSON(h.HandleObject, req)

	require.Equal(t, http.StatusOK, rec.Code)
	var resp struct {
		SpeedFactor         float64 `json:"speed_factor"`
		BatchSize           int     `json:"batch_size"`
		NotifyEnabledGlobal bool    `json:"notify_enabled_global"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.InDelta(t, 0.3, resp.SpeedFactor, 0.0001, "дефолт = скорость игрока, спека §2.4")
	require.Equal(t, 2000, resp.BatchSize)
	require.False(t, resp.NotifyEnabledGlobal, "глобальный рубильник выключен по умолчанию (спека 26a.1 §7.2)")
}

func TestAdminNPCSettingsPatch(t *testing.T) {
	h, _ := newAdminNPCHarness(t)

	req := httptest.NewRequest(http.MethodPatch, "/admin/npc/settings", strings.NewReader(`{"speed_factor":0.5,"batch_size":500,"notify_enabled_global":true}`))
	rec := execJSON(h.HandleObject, req)

	require.Equal(t, http.StatusOK, rec.Code)
	var resp struct {
		SpeedFactor         float64 `json:"speed_factor"`
		BatchSize           int     `json:"batch_size"`
		NotifyEnabledGlobal bool    `json:"notify_enabled_global"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.InDelta(t, 0.5, resp.SpeedFactor, 0.0001)
	require.Equal(t, 500, resp.BatchSize)
	require.True(t, resp.NotifyEnabledGlobal, "рубильник применяется сразу (спека 26a.1 §7.5)")
}

func TestAdminNPCSettingsPatchInvalid(t *testing.T) {
	h, _ := newAdminNPCHarness(t)

	req := httptest.NewRequest(http.MethodPatch, "/admin/npc/settings", strings.NewReader(`{"speed_factor":-1}`))
	rec := execJSON(h.HandleObject, req)
	require.Equal(t, http.StatusBadRequest, rec.Code)
}

// ==================== GET /api/npc/positions ====================

func TestAdminNPCPositionsEmpty(t *testing.T) {
	h, _ := newAdminNPCHarness(t)

	req := httptest.NewRequest(http.MethodGet, "/api/npc/positions", nil)
	rec := execJSON(h.Positions, req)

	require.Equal(t, http.StatusOK, rec.Code)
	var resp struct {
		Positions []interface{} `json:"positions"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Empty(t, resp.Positions, "позиций нет — пустой массив, не null")
}

// Позиция агента несёт race_id (спека 2026-09-23 §7.2): клиент выбирает
// корабль расы (spriteForAgent(race_id, id)).
func TestAdminNPCPositionsCarriesRaceID(t *testing.T) {
	h, _ := newAdminNPCHarness(t)
	h.manager.SetPositions([]npc.InterpolatedPosition{
		{ID: "a1", Status: models.NPCAgentStatusFlying, X: 1, Y: 2, RaceID: "coastal"},
	})

	req := httptest.NewRequest(http.MethodGet, "/api/npc/positions", nil)
	rec := execJSON(h.Positions, req)

	require.Equal(t, http.StatusOK, rec.Code)
	var resp struct {
		Positions []struct {
			ID     string `json:"id"`
			RaceID string `json:"race_id"`
		} `json:"positions"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Len(t, resp.Positions, 1)
	require.Equal(t, "coastal", resp.Positions[0].RaceID)
}