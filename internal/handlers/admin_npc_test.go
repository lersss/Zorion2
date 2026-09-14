// internal/handlers/admin_npc_test.go
// Тесты ручек NPC-агентов (спека 20a.1 §8): список, создание, удаление,
// частичное обновление, настройки менеджера, позиции для карты.
package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"

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

// newAdminNPCHarness — sqlmock-БД + хендлеры NPC с менеджером без сетки.
func newAdminNPCHarness(t *testing.T) (*AdminNPCHandlers, sqlmock.Sqlmock) {
	t.Helper()
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	manager := npc.NewManager(npcFakeStore{}, npcFakeWorlds{}, npc.DefaultSettings())
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

	mock.ExpectQuery(`SELECT id, name, status, current_world_id, from_world_id, target_world_id, depart_at, arrive_at, notify_enabled, last_observed_at, created_at, updated_at FROM npc_agents`).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "name", "status", "current_world_id", "from_world_id", "target_world_id", "depart_at", "arrive_at", "notify_enabled", "last_observed_at", "created_at", "updated_at",
		}).AddRow(agentID, "Наблюдатель-1", "flying", "w1", "w1", "w2", now(), now().Add(time.Minute), true, nil, now(), now()))

	req := httptest.NewRequest(http.MethodGet, "/admin/npc", nil)
	rec := execJSON(h.HandleCollection, req)

	require.Equal(t, http.StatusOK, rec.Code)
	var resp struct {
		Agents []struct {
			ID     string `json:"id"`
			Name   string `json:"name"`
			Status string `json:"status"`
		} `json:"agents"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Len(t, resp.Agents, 1)
	require.Equal(t, "Наблюдатель-1", resp.Agents[0].Name)
	require.Equal(t, "flying", resp.Agents[0].Status)
	require.NoError(t, mock.ExpectationsWereMet())
}

// ==================== POST /admin/npc ====================

func TestAdminNPCCreateWithStartWorld(t *testing.T) {
	h, mock := newAdminNPCHarness(t)

	mock.ExpectQuery(`SELECT id, name, coord_x, coord_y, spectral_class, temperature, created_at, updated_at FROM worlds WHERE id = \$1`).
		WithArgs("22222222-2222-2222-2222-222222222222").
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "name", "coord_x", "coord_y", "spectral_class", "temperature", "created_at", "updated_at",
		}).AddRow("22222222-2222-2222-2222-222222222222", "Sirius", 0, 0, "A", 10000, now(), now()))
	mock.ExpectExec(`INSERT INTO npc_agents \(id, name, status, current_world_id, from_world_id, target_world_id, depart_at, arrive_at, notify_enabled, last_observed_at, created_at, updated_at\) VALUES \(\$1, \$2, \$3, \$4, \$5, \$6, \$7, \$8, \$9, \$10, NOW\(\), NOW\(\)\)`).
		WithArgs(sqlmock.AnyArg(), "Наблюдатель-1", "idle", "22222222-2222-2222-2222-222222222222", nil, nil, nil, nil, true, nil).
		WillReturnResult(sqlmock.NewResult(0, 1))

	body := `{"name":"Наблюдатель-1","start_world_id":"22222222-2222-2222-2222-222222222222"}`
	req := httptest.NewRequest(http.MethodPost, "/admin/npc", strings.NewReader(body))
	rec := execJSON(h.HandleCollection, req)

	require.Equal(t, http.StatusCreated, rec.Code)
	var resp struct {
		ID     string `json:"id"`
		Status string `json:"status"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Equal(t, "idle", resp.Status, "создание ставит агента в idle (спека §8)")
	require.NoError(t, mock.ExpectationsWereMet())
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

	mock.ExpectQuery(`SELECT id, name, coord_x, coord_y, spectral_class, temperature, created_at, updated_at FROM worlds WHERE id = \$1`).
		WithArgs("22222222-2222-2222-2222-222222222222").
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "name", "coord_x", "coord_y", "spectral_class", "temperature", "created_at", "updated_at",
		}))

	body := `{"name":"A","start_world_id":"22222222-2222-2222-2222-222222222222"}`
	req := httptest.NewRequest(http.MethodPost, "/admin/npc", strings.NewReader(body))
	rec := execJSON(h.HandleCollection, req)
	require.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestAdminNPCCreateNoWorlds(t *testing.T) {
	// Без start_world_id и без сетки миров (менеджер RandomWorld → false) — 400.
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
	mock.ExpectQuery(`SELECT id, name, status, current_world_id, from_world_id, target_world_id, depart_at, arrive_at, notify_enabled, last_observed_at, created_at, updated_at FROM npc_agents WHERE id = \$1`).
		WithArgs(agentID).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "name", "status", "current_world_id", "from_world_id", "target_world_id", "depart_at", "arrive_at", "notify_enabled", "last_observed_at", "created_at", "updated_at",
		}).AddRow(agentID, "Новое имя", "idle", "w1", nil, nil, nil, nil, false, nil, now(), now()))

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

// ==================== НАСТРОЙКИ /admin/npc/settings ====================

func TestAdminNPCSettingsGet(t *testing.T) {
	h, _ := newAdminNPCHarness(t)

	req := httptest.NewRequest(http.MethodGet, "/admin/npc/settings", nil)
	rec := execJSON(h.HandleObject, req)

	require.Equal(t, http.StatusOK, rec.Code)
	var resp struct {
		SpeedFactor float64 `json:"speed_factor"`
		BatchSize   int     `json:"batch_size"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.InDelta(t, 0.3, resp.SpeedFactor, 0.0001, "дефолт = скорость игрока, спека §2.4")
	require.Equal(t, 2000, resp.BatchSize)
}

func TestAdminNPCSettingsPatch(t *testing.T) {
	h, _ := newAdminNPCHarness(t)

	req := httptest.NewRequest(http.MethodPatch, "/admin/npc/settings", strings.NewReader(`{"speed_factor":0.5,"batch_size":500}`))
	rec := execJSON(h.HandleObject, req)

	require.Equal(t, http.StatusOK, rec.Code)
	var resp struct {
		SpeedFactor float64 `json:"speed_factor"`
		BatchSize   int     `json:"batch_size"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.InDelta(t, 0.5, resp.SpeedFactor, 0.0001)
	require.Equal(t, 500, resp.BatchSize)
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