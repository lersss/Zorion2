-- 005_economy_tables.sql — пустой стаб с первого коммита, поэтому таблицы
-- экономики на чистой базе не создавались ни одной миграцией (в dev они
-- появились вручную). Эта миграция закрывает пробел идемпотентно:
-- на dev-БД (таблицы уже есть) ничего не делает.

CREATE TABLE IF NOT EXISTS settlements (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    planet_id   uuid NOT NULL REFERENCES planets(id) ON DELETE CASCADE,
    level       integer NOT NULL DEFAULT 1,
    population  integer NOT NULL DEFAULT 0,
    capacity    integer NOT NULL DEFAULT 1000,
    stability   integer NOT NULL DEFAULT 50,
    created_at  timestamptz DEFAULT now(),
    updated_at  timestamptz DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_settlements_planet_id ON settlements (planet_id);

CREATE TABLE IF NOT EXISTS factories (
    id             uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    planet_id      uuid NOT NULL REFERENCES planets(id) ON DELETE CASCADE,
    name           text NOT NULL,
    type           text NOT NULL,
    input_resource text NOT NULL,
    output_product text NOT NULL,
    quality        integer NOT NULL DEFAULT 50,
    status         text NOT NULL DEFAULT 'active',
    created_at     timestamptz DEFAULT now(),
    updated_at     timestamptz DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_factories_planet_id ON factories (planet_id);

CREATE TABLE IF NOT EXISTS goods_batches (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    planet_id    uuid NOT NULL REFERENCES planets(id) ON DELETE CASCADE,
    product_name text NOT NULL,
    quantity     integer NOT NULL DEFAULT 0,
    quality      integer NOT NULL DEFAULT 50,
    producer_id  uuid,
    produced_at  timestamptz DEFAULT now(),
    created_at   timestamptz DEFAULT now(),
    expires_at   timestamptz
);
CREATE INDEX IF NOT EXISTS idx_goods_batches_planet_id ON goods_batches (planet_id);