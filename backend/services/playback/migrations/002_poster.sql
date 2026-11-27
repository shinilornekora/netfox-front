-- Ревизия постера из каталога: по ней строится адрес картинки в списках
-- «Продолжить», «История» и «Смотреть позже».
ALTER TABLE title_replica ADD COLUMN poster_rev bigint NOT NULL DEFAULT 0;
