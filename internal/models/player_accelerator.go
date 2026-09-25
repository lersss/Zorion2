package models

import "time"

// PlayerAcceleratorState — состояние отката ускорителя игрока (таблица
// player_accelerator, спека ускорителя §3.3): время последнего применения и
// снимок ПРИМЕНЁННОГО отката (минуты). Строки нет → оба поля nil (ещё не
// пользовался / откат не применялся).
type PlayerAcceleratorState struct {
	UserID          string
	LastBoostAt     *time.Time
	LastCooldownMin *int
}

// BoostActive — «на текущем сегменте действует ускорение» (спека §3.3): один
// усечённый момент — last_boost_at.UnixMilli() == segmentStart.UnixMilli().
// Сравнение именно по UnixMilli: time.Time из памяти несёт монотонные часы,
// БД — нет. Любая другая причина сегмента (новый /travel, разворот 61a,
// Restore небустнутого сегмента) даёт другой момент → равенство ложно.
func BoostActive(lastBoostAt *time.Time, segmentStart time.Time) bool {
	if lastBoostAt == nil {
		return false
	}
	return lastBoostAt.UnixMilli() == segmentStart.UnixMilli()
}

// CooldownRemaining — остаток отката (§3.3): now до last_boost_at +
// last_cooldown_min. Строки нет / откат не применялся / истёк → 0 (готов).
func CooldownRemaining(lastBoostAt *time.Time, lastCooldownMin *int, now time.Time) time.Duration {
	if lastBoostAt == nil || lastCooldownMin == nil {
		return 0
	}
	ready := lastBoostAt.Add(time.Duration(*lastCooldownMin) * time.Minute)
	if !now.Before(ready) {
		return 0
	}
	return ready.Sub(now)
}
