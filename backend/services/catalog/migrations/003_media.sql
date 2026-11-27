-- Имя видеофайла в хранилище плеера (MEDIA_DIR у playback): у фильма —
-- в карточке, у сериала — у каждой серии. Пустое — показывать демо-ролик.
ALTER TABLE titles ADD COLUMN media text NOT NULL DEFAULT '' CHECK (media = '' OR media ~ '^[a-z0-9][a-z0-9._-]{0,120}\.mp4$');
ALTER TABLE episodes ADD COLUMN media text NOT NULL DEFAULT '' CHECK (media = '' OR media ~ '^[a-z0-9][a-z0-9._-]{0,120}\.mp4$');
