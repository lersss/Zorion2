// internal/repository/effect_repository.go
// SQL-доступ к каталогу типов эффектов и их действующим эффектам (спека
// 2026-09-22-эффекты-снабжения-задержка-голод §3.1/§7.4/§19). Тот же паттерн,
// что goods_repository.go: каждая мутация — транзакция с pg_advisory_xact_lock
// (один каталог эффектов, одна мутация за раз); ошибки — ErrCatalog.
package repository

import (
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"zorion/internal/goodsstudio/graph"
)

// effectCatalogLockKey — фиксированный ключ pg_advisory_xact_lock каталога
// эффектов: одна мутация за раз (закрывает TOCTOU «проверил привязки → удалил»).
const effectCatalogLockKey = 0x45464643 // "EFFC"

// EffectTypeRow — тип эффекта из БД (спека §3.1): params.curve — ссылка на
// компоненту «Балансировки»; recovery в типе НЕ хранится (§7.1/§7.4).
type EffectTypeRow struct {
	ID        int64
	Name      string
	NameNorm  string
	Impact    string
	Curve     string
	CreatedAt time.Time
}

// EffectRepository — доступ к каталогу типов эффектов и действующим эффектам.
type EffectRepository struct {
	db *sql.DB
}

func NewEffectRepository(db *sql.DB) *EffectRepository {
	return &EffectRepository{db: db}
}

// beginMutation — транзакция мутации каталога эффектов с advisory lock.
func (r *EffectRepository) beginMutation() (*sql.Tx, error) {
	tx, err := r.db.Begin()
	if err != nil {
		return nil, err
	}
	if _, err := tx.Exec(`SELECT pg_advisory_xact_lock($1)`, effectCatalogLockKey); err != nil {
		tx.Rollback()
		return nil, err
	}
	return tx, nil
}

// effectParamsJSON — params типа эффекта: только ссылка curve (§3.1). Пустая
// строка — пустой объект (тип без кривой: сила 0, лог curve_unknown при резолве).
func effectParamsJSON(curve string) string {
	if curve == "" {
		return "{}"
	}
	b, err := json.Marshal(map[string]string{"curve": curve})
	if err != nil {
		return "{}"
	}
	return string(b)
}

// EffectTypes — каталог типов эффектов (студия, §7.4).
func (r *EffectRepository) EffectTypes() ([]EffectTypeRow, error) {
	rows, err := r.db.Query(
		`SELECT id, name, name_norm, impact, COALESCE(params->>'curve', ''), created_at
		 FROM effect_types ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []EffectTypeRow{}
	for rows.Next() {
		var e EffectTypeRow
		if err := rows.Scan(&e.ID, &e.Name, &e.NameNorm, &e.Impact, &e.Curve, &e.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// CreateEffectType — тип эффекта студии (§7.4): имя, impact (открытый набор,
// §5.4), params.curve (ссылка). Пустое имя/impact → 400; дубликат
// нормализованного имени → 409.
func (r *EffectRepository) CreateEffectType(name, impact, curve string) (EffectTypeRow, error) {
	tx, err := r.beginMutation()
	if err != nil {
		return EffectTypeRow{}, err
	}
	defer tx.Rollback()

	name = strings.TrimSpace(name)
	if name == "" {
		return EffectTypeRow{}, errCatalog(400, "имя пустое")
	}
	if strings.TrimSpace(impact) == "" {
		return EffectTypeRow{}, errCatalog(400, "impact обязателен")
	}
	var exists bool
	if err := tx.QueryRow(`SELECT EXISTS(SELECT 1 FROM effect_types WHERE name_norm = $1)`,
		graph.NormalizeName(name)).Scan(&exists); err != nil {
		return EffectTypeRow{}, err
	}
	if exists {
		return EffectTypeRow{}, errCatalog(409, "тип эффекта с таким именем уже есть")
	}
	var e EffectTypeRow
	if err := tx.QueryRow(
		`INSERT INTO effect_types (name, name_norm, impact, params) VALUES ($1, $2, $3, $4)
		 RETURNING id, name, name_norm, impact, COALESCE(params->>'curve', ''), created_at`,
		name, graph.NormalizeName(name), impact, effectParamsJSON(curve),
	).Scan(&e.ID, &e.Name, &e.NameNorm, &e.Impact, &e.Curve, &e.CreatedAt); err != nil {
		if isUniqueViolation(err) {
			return EffectTypeRow{}, errCatalog(409, "тип эффекта с таким именем уже есть")
		}
		return EffectTypeRow{}, err
	}
	if err := tx.Commit(); err != nil {
		return EffectTypeRow{}, err
	}
	return e, nil
}

// UpdateEffectType — частичное обновление типа (§7.4): nil-поле не трогается.
// Смена curve — jsonb_set на уровне ключа (чужие ключи params сохраняются).
// Не найдено → 404, дубликат имени → 409, пустое имя/impact → 400.
func (r *EffectRepository) UpdateEffectType(id int64, name, impact, curve *string) error {
	tx, err := r.beginMutation()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var exists bool
	if err := tx.QueryRow(`SELECT EXISTS(SELECT 1 FROM effect_types WHERE id = $1 FOR UPDATE)`, id).
		Scan(&exists); err != nil {
		return err
	}
	if !exists {
		return errCatalog(404, "тип эффекта не найден")
	}
	if name != nil {
		n := strings.TrimSpace(*name)
		if n == "" {
			return errCatalog(400, "имя пустое")
		}
		var dup bool
		if err := tx.QueryRow(`SELECT EXISTS(SELECT 1 FROM effect_types WHERE name_norm = $1 AND id <> $2)`,
			graph.NormalizeName(n), id).Scan(&dup); err != nil {
			return err
		}
		if dup {
			return errCatalog(409, "тип эффекта с таким именем уже есть")
		}
		if _, err := tx.Exec(`UPDATE effect_types SET name = $1, name_norm = $2, updated_at = NOW() WHERE id = $3`,
			n, graph.NormalizeName(n), id); err != nil {
			return err
		}
	}
	if impact != nil {
		if strings.TrimSpace(*impact) == "" {
			return errCatalog(400, "impact обязателен")
		}
		if _, err := tx.Exec(`UPDATE effect_types SET impact = $1, updated_at = NOW() WHERE id = $2`,
			*impact, id); err != nil {
			return err
		}
	}
	if curve != nil {
		if _, err := tx.Exec(
			`UPDATE effect_types SET params = jsonb_set(COALESCE(params, '{}'::jsonb), '{curve}', to_jsonb($1::text), true), updated_at = NOW() WHERE id = $2`,
			*curve, id); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// DeleteEffectType — удаление типа (§7.4): тип с действующими эффектами —
// 409 (RESTRICT; понятный 409 вместо сырой ошибки FK). Не найдено → 404.
func (r *EffectRepository) DeleteEffectType(id int64) error {
	tx, err := r.beginMutation()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var exists bool
	if err := tx.QueryRow(`SELECT EXISTS(SELECT 1 FROM effect_types WHERE id = $1 FOR UPDATE)`, id).
		Scan(&exists); err != nil {
		return err
	}
	if !exists {
		return errCatalog(404, "тип эффекта не найден")
	}
	var active int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM active_effects WHERE effect_type_id = $1`, id).Scan(&active); err != nil {
		return err
	}
	if active > 0 {
		return errCatalog(409, "тип используется действующими эффектами")
	}
	if _, err := tx.Exec(`DELETE FROM effect_types WHERE id = $1`, id); err != nil {
		return err
	}
	return tx.Commit()
}

// EffectBindingsCount — счётчики привязок типа (§7.4): действующие эффекты
// (active_effects) и типы-владельцы, чьи params.effects ссылаются на этот тип
// (ключ — name_norm). Не найдено → 404.
func (r *EffectRepository) EffectBindingsCount(id int64) (active int, producers int, err error) {
	// Оба счётчика — в ОДНОЙ транзакции под advisory-локом каталога (замечание
	// @reviewer): иначе между запросами состояние могло измениться и счётчики
	// разъехались бы. Тот же lock, что у прочих мутаций каталога.
	tx, err := r.beginMutation()
	if err != nil {
		return 0, 0, err
	}
	defer tx.Rollback()

	var nameNorm string
	err = tx.QueryRow(`SELECT name_norm FROM effect_types WHERE id = $1`, id).Scan(&nameNorm)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, 0, errCatalog(404, "тип эффекта не найден")
	}
	if err != nil {
		return 0, 0, err
	}
	if err = tx.QueryRow(`SELECT COUNT(*) FROM active_effects WHERE effect_type_id = $1`, id).Scan(&active); err != nil {
		return 0, 0, err
	}
	// Привязка «позиция → эффект» живёт в producer_types.params.effects
	// (значения — name_norm типов, §4.2); считаем владельцев с такой ссылкой.
	if err = tx.QueryRow(
		`SELECT COUNT(*) FROM producer_types pt WHERE EXISTS (
			SELECT 1 FROM jsonb_each_text(COALESCE(pt.params->'effects', '{}'::jsonb)) e
			WHERE e.value = $1)`, nameNorm).Scan(&producers); err != nil {
		return 0, 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, 0, err
	}
	return active, producers, nil
}

// SetSettlementEffectLoad — админ-инструмент «задать load вручную» (песочница
// F9, §6): перезапись нагрузки действующего эффекта поселения, базис load_at —
// now. Отрицательная нагрузка → 400; строки нет → 404.
//
// Блокировка — ТОТ ЖЕ КЛЮЧ, что у owner-прохода (pg_advisory_xact_lock(
// hashtext(settlement_id)), §4.5; замечание @reviewer): правка админа и расчёт
// сериализуются, иначе owner-транзакция перезаписала бы свежую нагрузку, либо
// админ — свежий расчёт.
func (r *EffectRepository) SetSettlementEffectLoad(settlementID string, effectTypeID int64, load float64) error {
	if load < 0 {
		return errCatalog(400, "нагрузка не может быть отрицательной")
	}
	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`SELECT pg_advisory_xact_lock(hashtext($1))`, settlementID); err != nil {
		return err
	}
	res, err := tx.Exec(
		`UPDATE active_effects SET load = $1, load_at = NOW(), updated_at = NOW()
		 WHERE owner_type = 'settlement' AND owner_id = $2 AND effect_type_id = $3`,
		load, settlementID, effectTypeID)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return errCatalog(404, "действующий эффект не найден")
	}
	return tx.Commit()
}
