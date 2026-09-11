-- Добавляем выбранную иконку корабля игрока
ALTER TABLE users ADD COLUMN ship_icon TEXT NOT NULL DEFAULT 'ship_strela.svg';