-- ============ Реплика каталога ============
CREATE TABLE title_replica (
    id           uuid PRIMARY KEY,
    name         text NOT NULL,
    kind         text NOT NULL,
    year         int NOT NULL,
    age_rating   int NOT NULL,
    status       text NOT NULL,
    duration_sec int NOT NULL,
    poster_hue   int NOT NULL DEFAULT 200,
    episodes     jsonb NOT NULL DEFAULT '[]',
    revision     bigint NOT NULL
);

-- ============ Где остановился зритель ============
-- Одна строка на (зритель, тайтл): для сериала — последняя серия.
-- client_ms — время на устройстве зрителя: «пинги» прогресса могут прийти
-- не по порядку (два устройства, повтор после обрыва), побеждает более
-- поздний по времени устройства, а не по времени прихода на сервер.
CREATE TABLE progress (
    user_id      uuid NOT NULL,
    title_id     uuid NOT NULL,
    episode_id   uuid,
    position_sec int NOT NULL CHECK (position_sec >= 0),
    duration_sec int NOT NULL CHECK (duration_sec > 0),
    finished     boolean NOT NULL DEFAULT false,
    client_ms    bigint NOT NULL,
    updated_at   timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, title_id)
);
CREATE INDEX progress_continue_idx ON progress (user_id, updated_at DESC) WHERE NOT finished;

-- ============ История просмотров: секционирование по месяцам ============
-- Самая быстрорастущая таблица системы и «горячий раздел» (РХД-5): почти
-- все записи и чтения касаются последнего месяца. Секции по месяцам дают:
-- маленькие индексы горячей секции, дешёвое удаление старой истории
-- (DROP секции вместо DELETE миллионов строк), partition pruning в запросах
-- с диапазоном дат.
CREATE TABLE views (
    id         uuid NOT NULL,
    user_id    uuid NOT NULL,
    title_id   uuid NOT NULL,
    episode_id uuid,
    via        text NOT NULL,   -- subscription | purchase
    started_at timestamptz NOT NULL,
    PRIMARY KEY (id, started_at)  -- ключ секционирования обязан входить в PK
) PARTITION BY RANGE (started_at);
CREATE INDEX views_user_idx ON views (user_id, started_at DESC, id DESC);
-- Страховочная секция: запись с датой вне заготовленных месяцев не упадёт.
CREATE TABLE views_default PARTITION OF views DEFAULT;

-- Создаёт секции на текущий и следующие n месяцев (идемпотентно).
CREATE FUNCTION ensure_view_partitions(months_ahead int) RETURNS int LANGUAGE plpgsql AS $$
DECLARE
    m date := date_trunc('month', now())::date;
    created int := 0;
    name text;
BEGIN
    FOR i IN 0..months_ahead LOOP
        name := format('views_%s', to_char(m, 'YYYY_MM'));
        IF to_regclass(name) IS NULL THEN
            EXECUTE format('CREATE TABLE %I PARTITION OF views FOR VALUES FROM (%L) TO (%L)',
                           name, m, (m + interval '1 month')::date);
            created := created + 1;
        END IF;
        m := (m + interval '1 month')::date;
    END LOOP;
    RETURN created;
END $$;
SELECT ensure_view_partitions(3);

-- ============ «Смотреть позже» ============
CREATE TABLE watchlist (
    user_id  uuid NOT NULL,
    title_id uuid NOT NULL,
    added_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, title_id)
);

-- ============ RLS: зритель видит только свои строки (РХД-2) ============
CREATE FUNCTION current_viewer() RETURNS text LANGUAGE sql STABLE AS
    $$ SELECT coalesce(current_setting('plenka.user_id', true), '') $$;

ALTER TABLE progress ENABLE ROW LEVEL SECURITY;
ALTER TABLE progress FORCE ROW LEVEL SECURITY;
CREATE POLICY own_progress ON progress USING (user_id::text = current_viewer()) WITH CHECK (user_id::text = current_viewer());

ALTER TABLE views ENABLE ROW LEVEL SECURITY;
ALTER TABLE views FORCE ROW LEVEL SECURITY;
CREATE POLICY own_views ON views USING (user_id::text = current_viewer()) WITH CHECK (user_id::text = current_viewer());

ALTER TABLE watchlist ENABLE ROW LEVEL SECURITY;
ALTER TABLE watchlist FORCE ROW LEVEL SECURITY;
CREATE POLICY own_watchlist ON watchlist USING (user_id::text = current_viewer()) WITH CHECK (user_id::text = current_viewer());

CREATE TABLE inbox (
    event_id   uuid PRIMARY KEY,
    event_type text NOT NULL,
    handled_at timestamptz NOT NULL DEFAULT now()
);
