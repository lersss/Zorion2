package repository

import (
	"context"
	"database/sql"
	"errors"

	"zorion/internal/models"
)

// ErrNoPingsLeft — у игрока не осталось импульсов разведки: вскрытие сектора
// отклонено, состояние не изменено (спека ускорителя §14.1/§14.4 R3).
var ErrNoPingsLeft = errors.New("route puzzle: no pings left")

// RoutePuzzleRepository — состояние сегментной задачи мини-игры «Прокладка
// маршрута» (таблица player_route_puzzle, спека ускорителя §14.1):
// детерминированное поле сегмента, вскрытые секторы и остаток импульсов.
// Таблица НЕ входит в truncateTables (состояние игрока, переживает очистку
// вселенной; FK → users).
type RoutePuzzleRepository struct {
	db *sql.DB
}

func NewRoutePuzzleRepository(db *sql.DB) *RoutePuzzleRepository {
	return &RoutePuzzleRepository{db: db}
}

const routePuzzleColumns = `user_id, kind, segment_hash, secret, layout, revealed, pings_left, created_at`

// Get — состояние задачи игрока. Строки нет → (nil, nil): отсутствие сегмента
// не ошибка (по образцу player_accelerator).
func (r *RoutePuzzleRepository) Get(ctx context.Context, userID, kind string) (*models.RoutePuzzle, error) {
	st := &models.RoutePuzzle{}
	err := r.db.QueryRowContext(ctx,
		`SELECT `+routePuzzleColumns+` FROM player_route_puzzle WHERE user_id = $1 AND kind = $2`,
		userID, kind,
	).Scan(&st.UserID, &st.Kind, &st.SegmentHash, &st.Secret, &st.Layout, &st.Revealed, &st.PingsLeft, &st.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return st, nil
}

// Ensure — атомарно привести строку задачи к сегменту state: вставить новую
// (или заменить устаревшую по segment_hash) и вернуть КАНОНИЧЕСКУЮ строку из
// БД. Фикс гонки Get+Replace (спека ускорителя §14.1): конкурентные запросы
// одного игрока при смене сегмента могли получить разные secret → разные доски.
// Условие `segment_hash IS DISTINCT FROM EXCLUDED.segment_hash` + row-lock PG
// делают первое обновление победителем (второй ждёт и его WHERE уже не проходит),
// поэтому все читают перечитыванием одну строку с одним secret. revealed
// сбрасывается — вскрытие старого сегмента не переносится.
func (r *RoutePuzzleRepository) Ensure(ctx context.Context, state *models.RoutePuzzle) (*models.RoutePuzzle, error) {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO player_route_puzzle (user_id, kind, segment_hash, secret, layout, revealed, pings_left, created_at)
		VALUES ($1, $2, $3, $4, $5, '[]'::jsonb, $6, NOW())
		ON CONFLICT (user_id, kind) DO UPDATE SET
			segment_hash = EXCLUDED.segment_hash,
			secret = EXCLUDED.secret,
			layout = EXCLUDED.layout,
			revealed = '[]'::jsonb,
			pings_left = EXCLUDED.pings_left,
			created_at = NOW()
		WHERE player_route_puzzle.segment_hash IS DISTINCT FROM EXCLUDED.segment_hash`,
		state.UserID, state.Kind, state.SegmentHash, state.Secret, state.Layout, state.PingsLeft,
	)
	if err != nil {
		return nil, err
	}
	return r.Get(ctx, state.UserID, state.Kind)
}

// Reveal — атомарно вскрыть сектор: списать 1 импульс (только если
// pings_left > 0, гейт в самом UPDATE) и дописать сектор в revealed. content —
// JSON-текст содержимого сектора; в revealed кладётся элемент
// {"sector":N,"content":...}. Импульсов нет / строки нет → ErrNoPingsLeft,
// состояние не изменено. Возвращает обновлённый revealed и остаток импульсов.
func (r *RoutePuzzleRepository) Reveal(ctx context.Context, userID, kind string, sectorIndex int, content string) ([]byte, int, error) {
	var revealed []byte
	var pingsLeft int
	err := r.db.QueryRowContext(ctx, `
		UPDATE player_route_puzzle
		SET pings_left = pings_left - 1,
		    revealed = revealed || jsonb_build_object('sector', $3::int, 'content', $4::jsonb)
		WHERE user_id = $1 AND kind = $2 AND pings_left > 0
		RETURNING revealed, pings_left`,
		userID, kind, sectorIndex, content,
	).Scan(&revealed, &pingsLeft)
	if err == sql.ErrNoRows {
		return nil, 0, ErrNoPingsLeft
	}
	if err != nil {
		return nil, 0, err
	}
	return revealed, pingsLeft, nil
}

// Delete — убрать состояние задачи игрока (идемпотентно).
func (r *RoutePuzzleRepository) Delete(ctx context.Context, userID, kind string) error {
	_, err := r.db.ExecContext(ctx,
		`DELETE FROM player_route_puzzle WHERE user_id = $1 AND kind = $2`,
		userID, kind,
	)
	return err
}
