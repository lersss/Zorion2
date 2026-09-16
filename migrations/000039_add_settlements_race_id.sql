-- settlements.race_id — раса поселения (спека 99.2.21 §2.3, §7.4).
-- NULL = легаси/люди. Две расы на одной планете = два ряда settlements
-- с разными race_id (существующее правило §13.2.1 «на одной планете может
-- быть несколько поселений»).
ALTER TABLE settlements ADD COLUMN IF NOT EXISTS race_id TEXT;