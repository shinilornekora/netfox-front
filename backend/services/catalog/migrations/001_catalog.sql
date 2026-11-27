-- Тайтл — фильм или сериал. Жизненный цикл: draft → published ⇄ withdrawn.
CREATE TABLE titles (
    id              uuid PRIMARY KEY,
    kind            text NOT NULL CHECK (kind IN ('film', 'series')),
    name            text NOT NULL CHECK (char_length(name) BETWEEN 1 AND 200),
    original_name   text NOT NULL DEFAULT '',
    year            int NOT NULL CHECK (year BETWEEN 1895 AND 2100),
    description     text NOT NULL DEFAULT '',
    age_rating      int NOT NULL CHECK (age_rating IN (0, 6, 12, 16, 18)),
    genres          text[] NOT NULL DEFAULT '{}',
    duration_sec    int NOT NULL DEFAULT 0 CHECK (duration_sec >= 0),
    in_subscription boolean NOT NULL DEFAULT true,
    price_kopecks   bigint NOT NULL DEFAULT 0 CHECK (price_kopecks >= 0),
    poster_hue      int NOT NULL DEFAULT 200 CHECK (poster_hue BETWEEN 0 AND 359),
    status          text NOT NULL DEFAULT 'draft' CHECK (status IN ('draft', 'published', 'withdrawn')),
    withdraw_reason text NOT NULL DEFAULT '',
    published_at    timestamptz,
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now(),
    revision        bigint NOT NULL DEFAULT 1,
    -- Сериал продаётся только по подписке; фильм — по подписке и/или покупкой.
    CONSTRAINT titles_series_not_for_sale CHECK (kind = 'film' OR price_kopecks = 0),
    -- Опубликованный тайтл должен быть доступен хоть одним способом.
    CONSTRAINT titles_published_reachable CHECK (status <> 'published' OR in_subscription OR price_kopecks > 0),
    -- Полнотекстовый поиск по-русски: «пират» найдёт «Пираты Карибского моря».
    search tsvector GENERATED ALWAYS AS (
        setweight(to_tsvector('russian', name), 'A') ||
        setweight(to_tsvector('simple', original_name), 'A') ||
        setweight(to_tsvector('russian', description), 'C')
    ) STORED
);
CREATE INDEX titles_search_idx ON titles USING gin (search);
CREATE INDEX titles_genres_idx ON titles USING gin (genres);
-- Индексы под курсорную пагинацию публичного каталога: сортировка + id
-- как «разрыватель ничьих», только опубликованные.
CREATE INDEX titles_pub_new_idx  ON titles (published_at DESC, id DESC) WHERE status = 'published';
CREATE INDEX titles_pub_year_idx ON titles (year DESC, id DESC) WHERE status = 'published';
CREATE INDEX titles_pub_name_idx ON titles (name, id) WHERE status = 'published';

CREATE TABLE episodes (
    id           uuid PRIMARY KEY,
    title_id     uuid NOT NULL REFERENCES titles (id) ON DELETE CASCADE,
    season       int NOT NULL CHECK (season BETWEEN 1 AND 99),
    number       int NOT NULL CHECK (number BETWEEN 1 AND 999),
    name         text NOT NULL CHECK (char_length(name) BETWEEN 1 AND 200),
    duration_sec int NOT NULL CHECK (duration_sec > 0),
    CONSTRAINT episodes_position_uq UNIQUE (title_id, season, number)
);

CREATE TABLE outbox (
    seq          bigserial PRIMARY KEY,
    event_id     uuid NOT NULL UNIQUE,
    topic        text NOT NULL,
    msg_key      text NOT NULL,
    body         jsonb NOT NULL,
    created_at   timestamptz NOT NULL DEFAULT now(),
    lease_owner  text,
    lease_until  timestamptz,
    attempts     int NOT NULL DEFAULT 0,
    last_error   text,
    sent_at      timestamptz
);
CREATE INDEX outbox_pending ON outbox (seq) WHERE sent_at IS NULL;
CREATE FUNCTION outbox_notify() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN PERFORM pg_notify('outbox', current_schema()); RETURN NULL; END $$;
CREATE TRIGGER outbox_notify AFTER INSERT ON outbox FOR EACH STATEMENT EXECUTE FUNCTION outbox_notify();
