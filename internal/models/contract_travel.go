package models

import "time"

// Параметры срока перелёта (спека 2026-09-22-контракт-перелёт-и-доска §4.3,
// заглушки «калибровать позже»). Единый источник для флоу игрока
// (internal/handlers) и агента (internal/npc): формула не дублируется.
const (
	// ContractDeadlineReserve — запас срока исполнения сверх времени полёта.
	ContractDeadlineReserve = 0.5
	// ContractOfferWindowMult — жизнь предложения на доске (пока open) как
	// множитель времени полёта на референсной тяге.
	ContractOfferWindowMult = 10.0
)

// TravelDuration — длительность полёта по расстоянию между мирами:
// dist·speedFactor секунд, минимум 3 секунды (спека §4.1; та же формула, что у
// /travel — calcTravelDuration). speedFactor — параметр двигателя (меньше —
// быстрее); EngineSpeedDefault — референс (самый медленный стартовый двигатель).
func TravelDuration(dist, speedFactor float64) time.Duration {
	d := time.Duration(dist*speedFactor) * time.Second
	if d < 3*time.Second {
		d = 3 * time.Second
	}
	return d
}

// TravelFlightTimeRef — время полёта на референсной тяге (speed_factor_ref =
// EngineSpeedDefault) — база срока и окна предложения перелёта (§4.3).
func TravelFlightTimeRef(dist float64) time.Duration {
	return TravelDuration(dist, EngineSpeedDefault)
}

// TravelDeadline — срок исполнения перелёта: время полёта на референсной тяге
// плюс запас (§4.3). Отсчитывается от взятия — expires_at перебазируется при
// взятии (капкан «взял перед истечением»), правило типа travel.
func TravelDeadline(dist float64) time.Duration {
	return time.Duration(float64(TravelFlightTimeRef(dist)) * (1 + ContractDeadlineReserve))
}

// TravelOfferWindow — жизнь предложения на доске, пока контракт open (§4.3);
// не путать со сроком исполнения (перебазируется при взятии).
func TravelOfferWindow(dist float64) time.Duration {
	return time.Duration(float64(TravelFlightTimeRef(dist)) * ContractOfferWindowMult)
}
