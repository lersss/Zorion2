package models

import "time"

// Пресет окна открытой supply-доли по умолчанию (спека
// 2026-09-23-контракт-ленивая-доска-пакет-и-снабжение §5.5, F4-A).
// НЕ КАЛИБРОВАНО: значение — баланс (@balancetester), не якорь.
const SupplyOfferWindowDefault = 24 * time.Hour

// SupplyOfferWindow — жизнь открытой доли поставки (пока контракт open):
// пресет владельца, без перебазирования (F4-A). Единый источник окна рядом с
// TravelOfferWindow: плоского «24 ч» на местах вызова не остаётся — дефолт
// подставляется здесь. Пустой пресет (≤ 0) → SupplyOfferWindowDefault.
// НЕ КАЛИБРОВАНО.
func SupplyOfferWindow(preset time.Duration) time.Duration {
	if preset <= 0 {
		return SupplyOfferWindowDefault
	}
	return preset
}

// SupplyDeadline — срок исполнения доли поставки. F4-A: у поставки срок НЕ
// перебазируется при взятии (правило §4.3 предшественника сохраняется
// буквально), поэтому срок = окно; отдельной формулы нет — алиас единого
// источника, чтобы значение бралось из одной точки.
func SupplyDeadline(preset time.Duration) time.Duration {
	return SupplyOfferWindow(preset)
}
