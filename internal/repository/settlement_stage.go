// internal/repository/settlement_stage.go
//
// Стадия поселения в owner-проходе (спека 2026-09-23-стадии-поселения-и-
// скорость-производства §4–§6): загрузка ладдеры/типов на пачку, оценка и
// переключение стадии на персистентном пути, добор веток набора новой стадии
// без удаления существующих, очистка складов, сброс нагрузки (БД + память) и
// корневая очистка сирот active_effects по ТИПУ ЭФФЕКТА (§5.4, S5).
//
// Пороги стадии — producer_types.params.stage (JSONB, §9.2 — миграции нет);
// текущая стадия — существующая колонка settlements.settlement_type_id.
package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log"

	"github.com/google/uuid"
	"github.com/lib/pq"

	"zorion/internal/economy/settlement"
	"zorion/internal/models"
)

const (
	// settlementRootTypeIDSQL — тип-родитель «Поселение» по id базового типа
	// поселения из generation_config (rename-safe, §4.3): резолв по id, НЕ по
	// name_norm. parent_id NULL (базовый тип сам — корень) → ладдера пуста.
	settlementRootTypeIDSQL = `SELECT parent_id FROM producer_types WHERE id = $1`

	// settlementStageLadderSQL — ладдера стадий: подтипы класса «Поселение»
	// (parent_id = тип-родитель), у которых задан params.stage (§4.3, M2).
	// Фильтр класса обязателен: запись вне класса с params.stage в ладдеру не
	// входит. Порядок — по id (детерминированный; итоговый порядок ладдеры
	// задаёт NewStageLadder по (Enter, ID)).
	settlementStageLadderSQL = `
		SELECT id, params->'stage' FROM producer_types
		WHERE params ? 'stage' AND parent_id = $1
		ORDER BY id`

	// producerTypesSelectSQL — нормы/привязки по типам из объединения
	// «владельцы ∪ ладдера» одним запросом на пачку (§3.3 п.3): после перехода
	// проход берёт настройки НОВОГО типа, а не только текущего. Пороги стадии
	// (params.stage) здесь не читаются — ладдеру строит отдельный предикат
	// класса (settlementStageLadderSQL), дублировать их незачем.
	producerTypesSelectSQL = `
		SELECT id, params->'eat', params->'effects'
		FROM producer_types WHERE id = ANY($1)`

	// settlementTypeWriteSQL — запись стадии в строку под FOR UPDATE (§5.1 п.1).
	settlementTypeWriteSQL = `
		UPDATE settlements SET settlement_type_id = $1, updated_at = NOW() WHERE id = $2`

	// settlementRecipesSelectSQL — набор рецептов типа и число заполненных
	// компонентов каждого (§5.1 п.3): ветка добирается только у рецептов с
	// ≥1 компонентом, остальные пропускаются со строкой в лог.
	settlementRecipesSelectSQL = `
		SELECT pr.recipe_id, COUNT(rc.component_id)
		FROM producer_recipes pr
		LEFT JOIN recipe_components rc ON rc.recipe_id = pr.recipe_id AND rc.component_id IS NOT NULL
		WHERE pr.producer_type_id = $1
		GROUP BY pr.recipe_id ORDER BY pr.recipe_id`

	// branchBuffersClearSQL — склады очищаются при переходе (§5.1 п.4): все
	// буферы обеих сторон у всех веток поселения (строки не удаляются — состав
	// строк добирает существующий topUpBranchInputs).
	branchBuffersClearSQL = `
		UPDATE settlement_branch_buffers SET amount = 0, updated_at = NOW()
		WHERE branch_id IN (SELECT id FROM settlement_branches WHERE settlement_id = $1)`

	// activeEffectsClearSQL — сброс базиса нагрузки владельца при переходе
	// (§5.1 п.5, в БД). В памяти сброс делает вызов: stored = nil.
	activeEffectsClearSQL = `
		DELETE FROM active_effects WHERE owner_type = 'settlement' AND owner_id = $1`

	// orphanEffectsDeleteSQL — корневая очистка сирот на КАЖДОМ персистентном
	// проходе (§5.4): ключ — ТИП ЭФФЕКТА (effect_type_id), НЕ source_position.
	// COALESCE обязателен (защита от NULL): пустой набор привязок удаляет все
	// строки владельца — верное поведение.
	orphanEffectsDeleteSQL = `
		DELETE FROM active_effects
		WHERE owner_type = 'settlement' AND owner_id = $1
		  AND effect_type_id <> ALL (COALESCE($2::bigint[], '{}'::bigint[]))`
)

// producerTypeMeta — настройки типа поселения из producer_types (§3.3 п.3):
// нормы (params.eat) и привязки (params.effects) по позициям. После перехода
// стадии проход берёт настройки НОВОГО типа.
type producerTypeMeta struct {
	EatByPosition     map[string]float64
	EffectsByPosition map[string]string
}

// ownerBatchData — данные, загруженные ОДИН раз на пачку поселений (§3.3):
// числа скорости пар «тип × рецепт» (rates; nil = число не объявлено), набор
// рецептов стадии (recipes — признак «не в наборе», §3.5), настройки типов
// (объединение «владельцы ∪ ладдера») и ладдера стадий.
type ownerBatchData struct {
	rates   map[int64]map[int64]*float64
	recipes map[int64]map[int64]bool
	types   map[int64]producerTypeMeta
	ladder  settlement.StageLadder
}

// stageJSON — params.stage в БД: пороги входа/выхода (в людях).
type stageJSON struct {
	Enter *float64 `json:"enter"`
	Exit  *float64 `json:"exit"`
}

// parseStage — params.stage строки типа в Stage (id — id типа). Пустое/`null`
// значение или невалидный JSON → (Stage{}, false): запись не входит в ладдеру.
func parseStage(id int64, raw []byte) (settlement.Stage, bool) {
	if len(raw) == 0 || string(raw) == "null" {
		return settlement.Stage{}, false
	}
	var s stageJSON
	if err := json.Unmarshal(raw, &s); err != nil {
		log.Printf("⚠️ stage: тип %d: params.stage не разобран: %v (stage_unknown)", id, err)
		return settlement.Stage{}, false
	}
	var enter, exit float64
	if s.Enter != nil {
		enter = *s.Enter
	}
	if s.Exit != nil {
		exit = *s.Exit
	}
	return settlement.Stage{ID: id, Enter: enter, Exit: exit}, true
}

// loadStageLadder — ладдера стадий на пачку (§4.3): резолв типа-родителя
// «Поселение» по id базового типа из generation_config (rename-safe), затем
// подтипы класса с params.stage. Не резолвится/пусто → пустая ладдера
// (переключений нет, безопасно). Возвращает ладдеру и id типов ладдеры (для
// объединения карт types/rates).
func (r *BranchRepository) loadStageLadder(ctx context.Context) (settlement.StageLadder, []int64, error) {
	rootID, err := r.settlementRootTypeID(ctx)
	if err != nil {
		return settlement.StageLadder{}, nil, err
	}
	if rootID == 0 {
		return settlement.NewStageLadder(nil), nil, nil
	}

	rows, err := r.db.QueryContext(ctx, settlementStageLadderSQL, rootID)
	if err != nil {
		return settlement.StageLadder{}, nil, err
	}
	defer rows.Close()
	var stages []settlement.Stage
	var ids []int64
	for rows.Next() {
		var id int64
		var raw []byte
		if err := rows.Scan(&id, &raw); err != nil {
			return settlement.StageLadder{}, nil, err
		}
		st, ok := parseStage(id, raw)
		if !ok {
			continue
		}
		stages = append(stages, st)
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return settlement.StageLadder{}, nil, err
	}
	return settlement.NewStageLadder(stages), ids, nil
}

// settlementRootTypeID — id типа-родителя «Поселение»: базовый тип поселения
// из generation_config.default_settlement_type_id → его parent_id (rename-safe,
// §4.3). Ключа нет / payload не число / у типа нет родителя → 0: ладдера пуста,
// переключений нет (безопасно). Каждый такой случай — строкой в лог, чтобы
// «стадии не переключаются» не выглядело загадкой.
func (r *BranchRepository) settlementRootTypeID(ctx context.Context) (int64, error) {
	var raw []byte
	err := r.db.QueryRowContext(ctx, defaultSettlementTypeSelectSQL, models.DefaultSettlementTypeIDKey).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		log.Printf("⚠️ stage: generation_config.%s не задан — ладдера стадий пуста (переключений нет)", models.DefaultSettlementTypeIDKey)
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	var typeID int64
	if err := json.Unmarshal(raw, &typeID); err != nil || typeID == 0 {
		log.Printf("⚠️ stage: generation_config.%s = %s не число — ладдера стадий пуста (переключений нет)", models.DefaultSettlementTypeIDKey, raw)
		return 0, nil
	}
	var rootID sql.NullInt64
	err = r.db.QueryRowContext(ctx, settlementRootTypeIDSQL, typeID).Scan(&rootID)
	if errors.Is(err, sql.ErrNoRows) {
		log.Printf("⚠️ stage: базовый тип %d не найден — ладдера стадий пуста (переключений нет)", typeID)
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	if !rootID.Valid {
		log.Printf("⚠️ stage: у базового типа %d нет parent_id — ладдера стадий пуста (переключений нет)", typeID)
		return 0, nil
	}
	return rootID.Int64, nil
}

// loadProducerTypes — нормы/привязки типов из объединения «владельцы ∪
// ладдера» одним запросом на пачку (§3.3 п.3). Пустой список — пустая карта.
func (r *BranchRepository) loadProducerTypes(ctx context.Context, typeIDs []int64) (map[int64]producerTypeMeta, error) {
	out := map[int64]producerTypeMeta{}
	if len(typeIDs) == 0 {
		return out, nil
	}
	rows, err := r.db.QueryContext(ctx, producerTypesSelectSQL, pq.Array(typeIDs))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		var eatRaw, effectsRaw []byte
		if err := rows.Scan(&id, &eatRaw, &effectsRaw); err != nil {
			return nil, err
		}
		meta := producerTypeMeta{}
		if len(eatRaw) > 0 {
			if err := json.Unmarshal(eatRaw, &meta.EatByPosition); err != nil {
				return nil, err
			}
		}
		if len(effectsRaw) > 0 {
			if err := json.Unmarshal(effectsRaw, &meta.EffectsByPosition); err != nil {
				return nil, err
			}
		}
		out[id] = meta
	}
	return out, rows.Err()
}

// applyStageTransition — переход поселения на новую стадию одной транзакцией
// (§5.1): запись settlement_type_id; добор недостающих веток набора новой
// стадии (существующие, включая ручные админ-ветки, НЕ удаляются); очистка
// складов; сброс нагрузки в БД; подмена настроек стадии в памяти прохода.
// Ветки, чей рецепт уже есть у поселения, не дублируются (UNIQUE).
func (r *BranchRepository) applyStageTransition(ctx context.Context, tx *sql.Tx, o *OwnerSettlement, newTypeID int64, existing []*branchRecord, types map[int64]producerTypeMeta) error {
	existingRecipes := make(map[int64]bool, len(existing))
	for _, rec := range existing {
		existingRecipes[rec.branch.RecipeID] = true
	}

	if _, err := tx.ExecContext(ctx, settlementTypeWriteSQL, newTypeID, o.ID); err != nil {
		return err
	}

	rows, err := tx.QueryContext(ctx, settlementRecipesSelectSQL, newTypeID)
	if err != nil {
		return err
	}
	type recipeComponents struct {
		recipeID  int64
		component int
	}
	var recipes []recipeComponents
	for rows.Next() {
		var rc recipeComponents
		if err := rows.Scan(&rc.recipeID, &rc.component); err != nil {
			rows.Close()
			return err
		}
		recipes = append(recipes, rc)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}

	for _, rc := range recipes {
		if rc.component == 0 {
			log.Printf("🌿 стадия %d: рецепт %d без заполненных компонентов — ветка не создана (settlement %s)", newTypeID, rc.recipeID, o.ID)
			continue
		}
		if existingRecipes[rc.recipeID] {
			continue
		}
		if err := r.CreateBranchTx(ctx, tx, uuid.New().String(), o.ID, rc.recipeID); err != nil {
			return err
		}
		log.Printf("🌿 стадия %d: поселению %s добавлена ветка рецепта %d (переход стадии)", newTypeID, o.ID, rc.recipeID)
	}

	if _, err := tx.ExecContext(ctx, branchBuffersClearSQL, o.ID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, activeEffectsClearSQL, o.ID); err != nil {
		return err
	}

	// Настройки новой стадии — в память прохода (§5.2): производство, потребность
	// и население этого прохода считаются по ОДНОЙ актуальной стадии.
	meta := types[newTypeID]
	o.SettlementTypeID = newTypeID
	o.EatByPosition = meta.EatByPosition
	o.EffectsByPosition = meta.EffectsByPosition
	return nil
}

// cleanupOrphanEffects — корневая очистка сирот active_effects владельца на
// персистентном проходе (§5.4): удаляются строки, чей тип эффекта не входит в
// текущие привязки стадии. $2 всегда строится вызывающим (пустая карта →
// пустой срез, не nil в SQL); COALESCE в SQL — защита в глубину.
func cleanupOrphanEffects(ctx context.Context, tx *sql.Tx, ownerID string, effectTypeIDs []int64) error {
	res, err := tx.ExecContext(ctx, orphanEffectsDeleteSQL, ownerID, pq.Array(effectTypeIDs))
	if err != nil {
		return err
	}
	if n, err := res.RowsAffected(); err == nil && n > 0 {
		log.Printf("⚠️ effect: поселение %s: удалено сирот active_effects: %d (orphan_cleanup)", ownerID, n)
	}
	return nil
}
