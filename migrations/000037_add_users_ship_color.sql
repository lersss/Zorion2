-- users.ship_color — выбранный цвет перекраски спрайта корабля (спека 61b §3.3/§7).
-- NULL = «Оригинал» (без перекраски); иначе hex из ShipColorPalette (9 цветов).
ALTER TABLE users ADD COLUMN ship_color TEXT;