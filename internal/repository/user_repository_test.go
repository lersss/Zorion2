// internal/repository/user_repository_test.go
// Тесты ролевой модели и инвариантов раздела «Пользователи» (спека 99.2.14
// §2, §7): роль в CRUD, List/Count с фильтрами, транзакционные проверки
// И1 (≥ 1 skycomposer) и И2 (нельзя менять/удалять себя).
package repository

import (
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"

	"zorion/internal/models"
)

// now — стабильная дата для строк sqlmock.
func now() time.Time {
	return time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC)
}

// ==================== CREATE ====================

// Роль не задана — по умолчанию player (спека §2: существующие учётки — player).
// Стартовая комплектация 77a (спека §3.3): ship_model_id='starter',
// equipment={radar:radar_1, scanner:scanner_1, engine:null}.
func TestCreateUserDefaultsToPlayerRole(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectExec(`INSERT INTO users (id, username, password_hash, email, agent_id, current_world_id, ship_icon, ship_model_id, equipment, role, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)`).
		WillReturnResult(sqlmock.NewResult(0, 1))

	user := &models.User{ID: "u1", Username: "bob", PasswordHash: "hash"}
	require.NoError(t, NewUserRepository(db).Create(user))
	require.NoError(t, mock.ExpectationsWereMet())
	require.Equal(t, models.RolePlayer, user.Role, "роль должна выставиться в player")
	require.NotNil(t, user.ShipModelID, "стартовая модель должна выставиться")
	require.Equal(t, models.StarterShipModelID, *user.ShipModelID)
	require.Equal(t, "radar_1", user.Equipment["radar"], "стартовый радар")
	require.Equal(t, "scanner_1", user.Equipment["scanner"], "стартовый сканер")
}

func TestCreateUserKeepsExplicitRole(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectExec(`INSERT INTO users (id, username, password_hash, email, agent_id, current_world_id, ship_icon, ship_model_id, equipment, role, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)`).
		WillReturnResult(sqlmock.NewResult(0, 1))

	user := &models.User{ID: "u1", Username: "sky", PasswordHash: "hash", Role: models.RoleSkycomposer}
	require.NoError(t, NewUserRepository(db).Create(user))
	require.NoError(t, mock.ExpectationsWereMet())
	require.Equal(t, models.RoleSkycomposer, user.Role)
}

// CreateWithAccount — игрок и его счёт в ОДНОЙ транзакции (спека
// 2026-09-22-деньги-и-эскроу §3.4): регистрация не оставляет игрока без счёта.
func TestCreateUserWithAccountSameTx(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectBegin()
	mock.ExpectExec(`INSERT INTO users \(id, username, password_hash, email, agent_id, current_world_id, ship_icon, ship_model_id, equipment, role, created_at, updated_at\)\s*VALUES \(\$1, \$2, \$3, \$4, \$5, \$6, \$7, \$8, \$9, \$10, \$11, \$12\)`).
		WithArgs("u1", "bob", "hash", nil, nil, nil, sqlmock.AnyArg(), "starter", sqlmock.AnyArg(), "player", sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`INSERT INTO accounts \(owner_type, owner_id, balance, withdrawable, created_at, updated_at\)`).
		WithArgs("player", "u1", int64(models.PlayerBalanceSeed)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	user := &models.User{ID: "u1", Username: "bob", PasswordHash: "hash"}
	require.NoError(t, NewUserRepository(db).CreateWithAccount(user))
	require.NoError(t, mock.ExpectationsWereMet())
	require.Equal(t, models.RolePlayer, user.Role)
}

// Сбой вставки счёта откатывает транзакцию — пользователь не создаётся
// «без счёта» (атомарность §3.4).
func TestCreateUserWithAccountRollsBackOnAccountFailure(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectBegin()
	mock.ExpectExec(`INSERT INTO users \(id, username, password_hash`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`INSERT INTO accounts \(owner_type, owner_id, balance`).
		WillReturnError(errors.New("boom"))
	mock.ExpectRollback()

	user := &models.User{ID: "u1", Username: "bob", PasswordHash: "hash"}
	require.Error(t, NewUserRepository(db).CreateWithAccount(user))
	require.NoError(t, mock.ExpectationsWereMet())
}

// ==================== GET ====================

func TestGetByUsernameScansRole(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectQuery(`SELECT id, username, password_hash, email, agent_id, current_world_id, ship_icon, ship_color, ship_model_id, equipment, role, created_at, updated_at FROM users WHERE username = \$1`).
		WithArgs("bob").
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "username", "password_hash", "email", "agent_id", "current_world_id", "ship_icon", "ship_color", "ship_model_id", "equipment", "role", "created_at", "updated_at",
		}).AddRow("u1", "bob", "hash", nil, nil, nil, "ship_strela.svg", nil, nil, nil, "admin", now(), now()))

	u, err := NewUserRepository(db).GetByUsername("bob")
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
	require.NotNil(t, u)
	require.Equal(t, models.RoleAdmin, u.Role)
}

// ==================== LIST / COUNT ====================

func TestCountUsersWithFilters(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM users WHERE \(username ILIKE.*OR email ILIKE.*AND role = \$2`).
		WithArgs("bo", "admin").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(3))

	n, err := NewUserRepository(db).Count("bo", "admin")
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
	require.Equal(t, 3, n)
}

func TestCountUsersNoFilters(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectQuery(`SELECT COUNT(*) FROM users`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(42))

	n, err := NewUserRepository(db).Count("", "")
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
	require.Equal(t, 42, n)
}

func TestListUsersPagination(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectQuery(`FROM users WHERE \(username ILIKE.*ORDER BY created_at DESC, id LIMIT \$2 OFFSET \$3`).
		WithArgs("bo", 20, 0).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "username", "password_hash", "email", "agent_id", "current_world_id", "ship_icon", "ship_color", "ship_model_id", "equipment", "role", "created_at", "updated_at",
		}).AddRow("u1", "bob", "hash", nil, nil, nil, "ship_strela.svg", nil, nil, nil, "player", now(), now()))

	users, err := NewUserRepository(db).List("bo", "", 1, 20)
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
	require.Len(t, users, 1)
	require.Equal(t, models.RolePlayer, users[0].Role)
}

// ==================== UPDATE ROLE (И1/И2) ====================

// Успешная смена роли: player → admin.
func TestUpdateRoleSuccess(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT role FROM users WHERE id = \$1 FOR UPDATE`).
		WithArgs("u1").
		WillReturnRows(sqlmock.NewRows([]string{"role"}).AddRow("player"))
	mock.ExpectExec(`UPDATE users SET role = \$1, updated_at = NOW\(\) WHERE id = \$2`).
		WithArgs("admin", "u1").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	require.NoError(t, NewUserRepository(db).UpdateRole("u1", models.RoleAdmin, "caller"))
	require.NoError(t, mock.ExpectationsWereMet())
}

// И2: skycomposer не может поменять роль самому себе.
func TestUpdateRoleSelfChangeForbidden(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT role FROM users WHERE id = \$1 FOR UPDATE`).
		WithArgs("u1").
		WillReturnRows(sqlmock.NewRows([]string{"role"}).AddRow("skycomposer"))
	mock.ExpectRollback()

	err = NewUserRepository(db).UpdateRole("u1", models.RolePlayer, "u1")
	require.ErrorIs(t, err, ErrSelfChange)
	require.NoError(t, mock.ExpectationsWereMet())
}

// И1: снять последнего skycomposer нельзя.
func TestUpdateRoleLastSkycomposerForbidden(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT role FROM users WHERE id = \$1 FOR UPDATE`).
		WithArgs("sky1").
		WillReturnRows(sqlmock.NewRows([]string{"role"}).AddRow("skycomposer"))
	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM users WHERE role = \$1`).
		WithArgs("skycomposer").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectRollback()

	err = NewUserRepository(db).UpdateRole("sky1", models.RolePlayer, "caller")
	require.ErrorIs(t, err, ErrLastSkycomposer)
	require.NoError(t, mock.ExpectationsWereMet())
}

// И1: снять НЕ последнего skycomposer можно (остаётся ≥ 1).
func TestUpdateRoleNotLastSkycomposerAllowed(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT role FROM users WHERE id = \$1 FOR UPDATE`).
		WithArgs("sky1").
		WillReturnRows(sqlmock.NewRows([]string{"role"}).AddRow("skycomposer"))
	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM users WHERE role = \$1`).
		WithArgs("skycomposer").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(2))
	mock.ExpectExec(`UPDATE users SET role = \$1, updated_at = NOW\(\) WHERE id = \$2`).
		WithArgs("player", "sky1").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	require.NoError(t, NewUserRepository(db).UpdateRole("sky1", models.RolePlayer, "caller"))
	require.NoError(t, mock.ExpectationsWereMet())
}

// Пользователь не найден — sql.ErrNoRows.
func TestUpdateRoleNotFound(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT role FROM users WHERE id = \$1 FOR UPDATE`).
		WithArgs("nope").
		WillReturnRows(sqlmock.NewRows([]string{"role"}))
	mock.ExpectRollback()

	err = NewUserRepository(db).UpdateRole("nope", models.RoleAdmin, "caller")
	require.ErrorIs(t, err, sql.ErrNoRows)
	require.NoError(t, mock.ExpectationsWereMet())
}

// Повышение до skycomposer не затрагивает И1 — проверки нет.
func TestUpdateRolePromoteToSkycomposer(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT role FROM users WHERE id = \$1 FOR UPDATE`).
		WithArgs("u1").
		WillReturnRows(sqlmock.NewRows([]string{"role"}).AddRow("admin"))
	mock.ExpectExec(`UPDATE users SET role = \$1, updated_at = NOW\(\) WHERE id = \$2`).
		WithArgs("skycomposer", "u1").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	require.NoError(t, NewUserRepository(db).UpdateRole("u1", models.RoleSkycomposer, "caller"))
	require.NoError(t, mock.ExpectationsWereMet())
}

// ==================== UPDATE PASSWORD ====================

func TestUpdatePasswordSuccess(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectExec(`UPDATE users SET password_hash = \$1, updated_at = NOW\(\) WHERE id = \$2`).
		WithArgs("newhash", "u1").
		WillReturnResult(sqlmock.NewResult(0, 1))

	require.NoError(t, NewUserRepository(db).UpdatePassword("u1", "newhash"))
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestUpdatePasswordNotFound(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectExec(`UPDATE users SET password_hash = \$1, updated_at = NOW\(\) WHERE id = \$2`).
		WithArgs("newhash", "nope").
		WillReturnResult(sqlmock.NewResult(0, 0))

	err = NewUserRepository(db).UpdatePassword("nope", "newhash")
	require.ErrorIs(t, err, sql.ErrNoRows)
	require.NoError(t, mock.ExpectationsWereMet())
}

// ==================== UPDATE SHIP COLOR (спека 61b §7) ====================

// Цвет из палитры пишется в users.ship_color.
func TestUpdateShipColorSuccess(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	color := "#ef4444"
	mock.ExpectExec(`UPDATE users SET ship_color = \$1, updated_at = NOW\(\) WHERE id = \$2`).
		WithArgs(color, "u1").
		WillReturnResult(sqlmock.NewResult(0, 1))

	require.NoError(t, NewUserRepository(db).UpdateShipColor("u1", &color))
	require.NoError(t, mock.ExpectationsWereMet())
}

// NULL («Оригинал») пишется как NULL.
func TestUpdateShipColorNull(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectExec(`UPDATE users SET ship_color = \$1, updated_at = NOW\(\) WHERE id = \$2`).
		WithArgs(nil, "u1").
		WillReturnResult(sqlmock.NewResult(0, 1))

	require.NoError(t, NewUserRepository(db).UpdateShipColor("u1", nil))
	require.NoError(t, mock.ExpectationsWereMet())
}

// ==================== DELETE (И1/И2) ====================

// Удаление обычного пользователя — без проверки skycomposer'ов.
func TestDeleteUserSuccess(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT role FROM users WHERE id = \$1 FOR UPDATE`).
		WithArgs("u1").
		WillReturnRows(sqlmock.NewRows([]string{"role"}).AddRow("player"))
	mock.ExpectExec(`DELETE FROM users WHERE id = \$1`).
		WithArgs("u1").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	require.NoError(t, NewUserRepository(db).Delete("u1", "caller"))
	require.NoError(t, mock.ExpectationsWereMet())
}

// И2: удалить самого себя нельзя.
func TestDeleteSelfForbidden(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT role FROM users WHERE id = \$1 FOR UPDATE`).
		WithArgs("u1").
		WillReturnRows(sqlmock.NewRows([]string{"role"}).AddRow("skycomposer"))
	mock.ExpectRollback()

	err = NewUserRepository(db).Delete("u1", "u1")
	require.ErrorIs(t, err, ErrSelfChange)
	require.NoError(t, mock.ExpectationsWereMet())
}

// И1: удалить последнего skycomposer нельзя.
func TestDeleteLastSkycomposerForbidden(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT role FROM users WHERE id = \$1 FOR UPDATE`).
		WithArgs("sky1").
		WillReturnRows(sqlmock.NewRows([]string{"role"}).AddRow("skycomposer"))
	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM users WHERE role = \$1`).
		WithArgs("skycomposer").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectRollback()

	err = NewUserRepository(db).Delete("sky1", "caller")
	require.ErrorIs(t, err, ErrLastSkycomposer)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestDeleteNotFound(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT role FROM users WHERE id = \$1 FOR UPDATE`).
		WithArgs("nope").
		WillReturnRows(sqlmock.NewRows([]string{"role"}))
	mock.ExpectRollback()

	err = NewUserRepository(db).Delete("nope", "caller")
	require.ErrorIs(t, err, sql.ErrNoRows)
	require.NoError(t, mock.ExpectationsWereMet())
}

// ==================== COUNT BY ROLE (бутстрап §5) ====================

func TestCountByRole(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectQuery(`SELECT COUNT(*) FROM users WHERE role = $1`).
		WithArgs("skycomposer").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

	n, err := NewUserRepository(db).CountByRole(models.RoleSkycomposer)
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
	require.Equal(t, 1, n)
}

// ==================== НАМЕРЕНИЕ КОМПОЗИТНОГО МАРШРУТА (спека 99.2.30) ====================

// SetPendingDestination (M2, спека 99.2.30 §3.5): запись намерения на
// 202-идемпотентном пути /travel (полёт уже идёт, позиция уже NULL).
func TestUserSetPendingDestination(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectExec(`UPDATE users SET pending_destination = \$1, updated_at = NOW\(\) WHERE id = \$2`).
		WithArgs(sqlmock.AnyArg(), "u1").
		WillReturnResult(sqlmock.NewResult(0, 1))

	err = NewUserRepository(db).SetPendingDestination("u1", &models.PendingDestination{
		WorldID: "w2", ObjectType: "satellite", ObjectID: "s1",
	})
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

// SetPendingDestination(nil) — очистка намерения на 202-пути без destination.
func TestUserSetPendingDestinationNil(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectExec(`UPDATE users SET pending_destination = \$1, updated_at = NOW\(\) WHERE id = \$2`).
		WithArgs(sqlmock.AnyArg(), "u1").
		WillReturnResult(sqlmock.NewResult(0, 1))

	err = NewUserRepository(db).SetPendingDestination("u1", nil)
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

// ClearPendingDestination (спека 99.2.30 §4.4): очистка намерения.
func TestUserClearPendingDestination(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectExec(`UPDATE users SET pending_destination = NULL, updated_at = NOW\(\) WHERE id = \$1`).
		WithArgs("u1").
		WillReturnResult(sqlmock.NewResult(0, 1))

	err = NewUserRepository(db).ClearPendingDestination("u1")
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}
