-- Видеофайл фильма (у серий — внутри episodes jsonb).
ALTER TABLE title_replica ADD COLUMN media text NOT NULL DEFAULT '';
