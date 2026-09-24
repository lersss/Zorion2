// internal/repository/market_repository.go
// Справочник предложений витрины локального рынка (спека
// 2026-09-24-магазин-модулей-локальный-рынок §5): market_offers — статичные
// строки «что продаётся и за сколько», бесконечный запас. Чтение — все
// строки / по id; живость поселения (population_exact > 0) — гейт ПОКУПКИ
// (§7.2 п.2), на чтении витрины не вызывается (живость сейчас не
// раскрывается, §7.1).
package repository

import (
	"database/sql"
	"fmt"

	"zorion/internal/models"
)

// MarketRepository — доступ к market_offers.
type MarketRepository struct {
	db *sql.DB
}

func NewMarketRepository(db *sql.DB) *MarketRepository {
	return &MarketRepository{db: db}
}

// ListOffers — все предложения витрины (порядок по id, стабильный).
func (r *MarketRepository) ListOffers() ([]models.MarketOffer, error) {
	rows, err := r.db.Query(`SELECT id, kind, item_id, price FROM market_offers ORDER BY id ASC`)
	if err != nil {
		return nil, fmt.Errorf("failed to query market offers: %w", err)
	}
	defer rows.Close()

	var offers []models.MarketOffer
	for rows.Next() {
		var o models.MarketOffer
		if err := rows.Scan(&o.ID, &o.Kind, &o.ItemID, &o.Price); err != nil {
			return nil, fmt.Errorf("failed to scan market offer: %w", err)
		}
		offers = append(offers, o)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate market offers: %w", err)
	}
	return offers, nil
}

// GetOfferByID — предложение по id (nil, если нет).
func (r *MarketRepository) GetOfferByID(id int64) (*models.MarketOffer, error) {
	var o models.MarketOffer
	err := r.db.QueryRow(
		`SELECT id, kind, item_id, price FROM market_offers WHERE id = $1`,
		id,
	).Scan(&o.ID, &o.Kind, &o.ItemID, &o.Price)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to query market offer: %w", err)
	}
	return &o, nil
}

// HasLiveSettlement — есть ли на планете живое поселение (population_exact > 0).
// Гейт ПОКУПКИ (§7.2 п.2); на чтении витрины не вызывается — живость сейчас
// не раскрывается (§7.1).
func (r *MarketRepository) HasLiveSettlement(planetID string) (bool, error) {
	var one int
	err := r.db.QueryRow(
		`SELECT 1 FROM settlements WHERE planet_id = $1 AND population_exact > 0 LIMIT 1`,
		planetID,
	).Scan(&one)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("failed to query live settlement: %w", err)
	}
	return true, nil
}