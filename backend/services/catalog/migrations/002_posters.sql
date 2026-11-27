-- Постер хранится в базе каталога: один файл на тайтл, до 3 МБ.
-- poster_rev — ревизия тайтла, на которой постер последний раз менялся:
-- он входит в URL (?v=), поэтому браузер и CDN могут кэшировать картинку
-- навсегда, а новая версия получает новый адрес.
ALTER TABLE titles ADD COLUMN poster_rev bigint NOT NULL DEFAULT 0;

CREATE TABLE posters (
    title_id     uuid PRIMARY KEY REFERENCES titles (id) ON DELETE CASCADE,
    content_type text NOT NULL CHECK (content_type IN ('image/jpeg', 'image/png', 'image/webp')),
    data         bytea NOT NULL CHECK (octet_length(data) <= 3 * 1024 * 1024),
    updated_at   timestamptz NOT NULL DEFAULT now()
);
