package repository

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math"
	"time"

	"github.com/lib/pq"

	"zorion/internal/economy/settlement"
	"zorion/internal/models"
)

type EconomyRepository struct {
	db *sql.DB
}

func NewEconomyRepository(db *sql.DB) *EconomyRepository {
	return &EconomyRepository{db: db}
}

// Settlement
func (r *EconomyRepository) CreateSettlement(s *models.Settlement) error {
	// Тип поселения — настоящая связь (спека итерации 4 §3.4): если не задан
	// вызывающим, берём дефолтный тип из generation_config (0 → NULL).
	if s.SettlementTypeID == 0 {
		typeID, err := ResolveDefaultSettlementTypeID(r.db)
		if err != nil {
			return err
		}
		s.SettlementTypeID = typeID
	}
	query := `INSERT INTO settlements (id, planet_id, population, population_exact, stability, computed_at, settlement_type_id, created_at, updated_at)
	          VALUES ($1, $2, $3, $4, $5, NOW(), $6, NOW(), NOW())`
	_, err := r.db.Exec(query, s.ID, s.PlanetID, s.Population, float64(s.Population), s.Stability, settlementTypeArg(s.SettlementTypeID))
	return err
}

// ResolveDefaultSettlementTypeID — id дефолтного типа поселения из
// generation_config по ключу models.DefaultSettlementTypeIDKey (решение
// создателя 2026-09-23: резолв по id, а НЕ по name_norm — переименование типа
// («Обычное поселение» → «Городок») иначе оставляло новые поселения без типа).
// Ключ пишут миграция 000075 (существующие БД) и сид каталога (свежая БД).
// Вызывается один раз на джоб генерации. Ключа нет или payload не число —
// 0 (без ошибки, но с WARN в лог, не тихий no-op): связь остаётся NULL, чтение
// применит фолбэк DefaultEatK (§3.2).
func ResolveDefaultSettlementTypeID(db *sql.DB) (int64, error) {
	var raw []byte
	err := db.QueryRow(
		`SELECT payload FROM generation_config WHERE key = $1`, models.DefaultSettlementTypeIDKey,
	).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		log.Printf("WARN: generation_config.%s не задан — новые поселения получат settlement_type_id = NULL (без потребностей/голода)",
			models.DefaultSettlementTypeIDKey)
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	var id int64
	if err := json.Unmarshal(raw, &id); err != nil {
		log.Printf("WARN: generation_config.%s: payload %q не число: %v — новые поселения получат settlement_type_id = NULL",
			models.DefaultSettlementTypeIDKey, raw, err)
		return 0, nil
	}
	return id, nil
}

// settlementTypeArg — id типа в параметр SQL: 0 (нет типа) → NULL (колонка
// nullable, §3.1), иначе число.
func settlementTypeArg(id int64) interface{} {
	if id == 0 {
		return nil
	}
	return id
}

// GetSettlementsByPlanetIDs — возвращает поселения планет,
// сгруппированные по planet_id. Пустой список — планета без поселений.
// LEFT JOIN producer_types несёт тип поселения и СЫРЫЕ структуры норм
// params->'eat' и привязок params->'effects' без COALESCE (спека итерации 4
// §3.3): отсутствие записи обязано приехать отсутствием ключа — фолбэк
// DefaultEatK применяет Go (§4.3). Ключ eat/effects — ПОЗИЦИЯ корзины
// (спека 2026-09-22-эффекты-снабжения §4.2).
func (r *EconomyRepository) GetSettlementsByPlanetIDs(planetIDs []string) (map[string][]models.Settlement, error) {
	if len(planetIDs) == 0 {
		return map[string][]models.Settlement{}, nil
	}

	query := `SELECT s.id, s.planet_id, s.population, s.population_exact, s.stability, s.computed_at,
	                 s.created_at, s.updated_at, s.race_id, s.settlement_type_id, pt.name,
	                 pt.params->'eat', pt.params->'effects'
	          FROM settlements s
	          LEFT JOIN producer_types pt ON pt.id = s.settlement_type_id
	          WHERE s.planet_id = ANY($1) ORDER BY s.created_at ASC`
	rows, err := r.db.Query(query, pqStringArray(planetIDs))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := map[string][]models.Settlement{}
	for rows.Next() {
		var s models.Settlement
		var raceID, typeName sql.NullString
		var typeID sql.NullInt64
		var eatRaw, effectsRaw []byte
		if err := rows.Scan(
			&s.ID, &s.PlanetID, &s.Population,
			&s.PopulationExact, &s.Stability, &s.ComputedAt,
			&s.CreatedAt, &s.UpdatedAt, &raceID,
			&typeID, &typeName, &eatRaw, &effectsRaw,
		); err != nil {
			return nil, err
		}
		s.RaceID = raceID.String
		s.SettlementTypeID = typeID.Int64
		s.TypeName = typeName.String
		if len(eatRaw) > 0 {
			var eat map[string]float64
			if err := json.Unmarshal(eatRaw, &eat); err != nil {
				return nil, fmt.Errorf("settlement type eat (%s): %w", s.ID, err)
			}
			s.EatByPosition = eat
		}
		if len(effectsRaw) > 0 {
			var effects map[string]string
			if err := json.Unmarshal(effectsRaw, &effects); err != nil {
				return nil, fmt.Errorf("settlement type effects (%s): %w", s.ID, err)
			}
			s.EffectsByPosition = effects
		}
		result[s.PlanetID] = append(result[s.PlanetID], s)
	}
	return result, rows.Err()
}

// RecomputeSettlementPopulation продвигает население поселения на момент now.
// Чек-точка для пересчёта уже загружена в s (population_exact, computed_at).
// Два пути (docs/gamedesign/18a_population_death.md, решение игрока
// 2026-09-13 — модель «правда на сервере, синк по событию»):
//   - «простой визит» (Δt < MinPersistInterval): население считается только
//     в памяти. Пересчёт — чистая функция от чек-точки (p0·exp(−λ·Δt)), запись
//     на хот-пате чтения не нужна, никаких запросов к БД, кроме уже сделанного.
//   - «событие» (Δt ≥ MinPersistInterval): чек-точка продвигается в БД
//     (транзакция, SELECT ... FOR UPDATE — конкурентная безопасность, два
//     одновременных события не исказят population_exact, AGENTS.md §0,
//     13_tiers_impl.md §13.13.4), чтобы сохранённое население не устаревало
//     для читателей без пересчёта (admin-stats, генератор фракций).
// В обоих путях в ответ попадает актуальное население и чек-точка на момент
// now, а r_per_sec/n_dead — для косметической экстраполяции на клиенте
// (λ-механизм убран, 99.2.13: всё изменение в r_per_sec).
// Если «событие» впервые видит обвал (чек-точка живая, population_exact >
// NDead, и next = 0), той же транзакцией создаётся запись лога «Вымерло»
// (18b §«Лог поселения»): INSERT ... ON CONFLICT DO NOTHING — анти-дубль на
// уровне БД (uq_settlement_log_extinct). Мёртвая чек-точка (population_exact
// ≤ NDead) запись не создаёт — бэкфилл отменён.
func (r *EconomyRepository) RecomputeSettlementPopulation(s *models.Settlement, input settlement.PlanetInput, now time.Time) (models.Settlement, error) {
	// Раса поселения → расовая R-модель (99.2.23 §2.2): одна точка — вход
	// получает RaceID из поселения; ChangeComponents/Recompute диспетчеризуют.
	input.RaceID = s.RaceID
	rPerSec := settlement.ChangeComponents(input)

	if now.Sub(s.ComputedAt) < settlement.MinPersistInterval {
		next := settlement.Recompute(input, s.PopulationExact, s.ComputedAt, now, s.CreatedAt)
		// Кламп записи int4 (99.2.23 §3.2): 2^31−1 перед приведением к int —
		// защита от «integer out of range» в settlements.population.
		s.Population = int(math.Round(math.Min(next, settlement.MaxInt4Population)))
		s.PopulationExact = next
		s.ComputedAt = now
		s.RPerSec = rPerSec
		s.NDead = settlement.NDead
		return *s, nil
	}

	tx, err := r.db.Begin()
	if err != nil {
		return models.Settlement{}, err
	}
	defer tx.Rollback()

	var stored models.Settlement
	err = tx.QueryRow(`
		SELECT id, planet_id, population, population_exact, stability, computed_at, created_at, updated_at, race_id
		FROM settlements WHERE id = $1 FOR UPDATE`, s.ID,
	).Scan(&stored.ID, &stored.PlanetID, &stored.Population, &stored.PopulationExact, &stored.Stability, &stored.ComputedAt, &stored.CreatedAt, &stored.UpdatedAt, &stored.RaceID)
	if err != nil {
		return models.Settlement{}, err
	}

	newExact := settlement.Recompute(input, stored.PopulationExact, stored.ComputedAt, now, stored.CreatedAt)
	// Кламп записи int4 (99.2.23 §3.2): 2^31−1 перед приведением к int —
	// защита от «integer out of range» в settlements.population.
	newPopulation := int(math.Round(math.Min(newExact, settlement.MaxInt4Population)))

	if _, err := tx.Exec(`
		UPDATE settlements SET population = $1, population_exact = $2, computed_at = $3, updated_at = NOW()
		WHERE id = $4`,
		newPopulation, newExact, now, stored.ID,
	); err != nil {
		return models.Settlement{}, err
	}

	// Обвал с живой чек-точки: население обнулилось целиком (next < NDead →
	// 0) — дата смерти вычислима, пишем «Вымерло» в лог той же транзакцией.
	// INSERT ... ON CONFLICT DO NOTHING: второй одновременный синк упирается
	// в uq_settlement_log_extinct и ничего не пишет (18b §«Анти-дубль и синк»).
	if stored.PopulationExact > settlement.NDead && newExact == 0 {
		deathAt, ok := settlement.DeathTime(stored.PopulationExact, rPerSec, stored.ComputedAt, stored.CreatedAt)
		if ok {
			cause := settlement.DeathCause(input)
			if _, err := tx.Exec(`
				INSERT INTO settlement_log (settlement_id, type, occurred_at, cause)
				VALUES ($1, 'extinct', $2, $3)
				ON CONFLICT DO NOTHING`,
				stored.ID, deathAt, cause,
			); err != nil {
				return models.Settlement{}, err
			}
		}
	}

	if err := tx.Commit(); err != nil {
		return models.Settlement{}, err
	}

	// SELECT ... FOR UPDATE читает только строку населения: тип поселения,
	// нормы (params.eat) и привязки (params.effects) переносим из прочитанного
	// поселения s — иначе путь «событие» вернул бы nil EatByPosition, и слой
	// потребности считал бы спрос по DefaultEatK (спека итерации 4 §3.3/§3.5;
	// спека 2026-09-22-эффекты-снабжения §4.2; T2/T6/T8/T19).
	stored.SettlementTypeID = s.SettlementTypeID
	stored.TypeName = s.TypeName
	stored.EatByPosition = s.EatByPosition
	stored.EffectsByPosition = s.EffectsByPosition
	stored.Population = newPopulation
	stored.PopulationExact = newExact
	stored.ComputedAt = now
	stored.RPerSec = rPerSec
	stored.NDead = settlement.NDead
	return stored, nil
}

// GetSettlementLogBySettlementIDs возвращает последние 3 записи лога на
// поселение, сгруппированные по settlement_id, сортировка по дате убывающая
// (18b §«UI»). Пустой список id — пустой результат, без запроса.
func (r *EconomyRepository) GetSettlementLogBySettlementIDs(ids []string) (map[string][]models.SettlementLogEntry, error) {
	if len(ids) == 0 {
		return map[string][]models.SettlementLogEntry{}, nil
	}

	query := `
		SELECT id, settlement_id, type, occurred_at, cause, created_at
		FROM (
			SELECT id, settlement_id, type, occurred_at, cause, created_at,
			       ROW_NUMBER() OVER (PARTITION BY settlement_id ORDER BY occurred_at DESC) AS rn
			FROM settlement_log
			WHERE settlement_id = ANY($1)
		) sub
		WHERE rn <= 3
		ORDER BY occurred_at DESC`
	rows, err := r.db.Query(query, pqStringArray(ids))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := map[string][]models.SettlementLogEntry{}
	for rows.Next() {
		var e models.SettlementLogEntry
		var cause sql.NullString
		if err := rows.Scan(
			&e.ID, &e.SettlementID, &e.Type, &e.OccurredAt, &cause, &e.CreatedAt,
		); err != nil {
			return nil, err
		}
		if cause.Valid {
			e.Cause = &cause.String
		}
		result[e.SettlementID] = append(result[e.SettlementID], e)
	}
	return result, rows.Err()
}

// pqStringArray — []string в тип для `= ANY($1)`. lib/pq сам кодирует
// []string как text[], но требует тип pq.Array.
func pqStringArray(ids []string) interface{} {
	return pq.Array(ids)
}
