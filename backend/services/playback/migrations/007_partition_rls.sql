-- Секции истории не наследуют политику RLS родителя: запрос прямо к
-- views_2026_11 обходил own_views. Теперь каждая секция получает ту же
-- политику с FORCE — и существующие, и создаваемые обслуживанием. Кроме
-- того, роль данных лишается прав на секции при старте сервиса
-- (pgkit.GrantApp): обращаться к истории она может только через views.
CREATE FUNCTION protect_view_partition(part text) RETURNS void LANGUAGE plpgsql AS $$
BEGIN
    EXECUTE format('ALTER TABLE %I ENABLE ROW LEVEL SECURITY', part);
    EXECUTE format('ALTER TABLE %I FORCE ROW LEVEL SECURITY', part);
    IF NOT EXISTS (SELECT 1 FROM pg_policies WHERE schemaname = current_schema()
                   AND tablename = part AND policyname = 'own_views') THEN
        EXECUTE format('CREATE POLICY own_views ON %I USING (user_id::text = current_viewer())
                        WITH CHECK (user_id::text = current_viewer())', part);
    END IF;
END $$;

CREATE OR REPLACE FUNCTION ensure_view_partitions(months_ahead int) RETURNS int LANGUAGE plpgsql AS $$
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
            PERFORM protect_view_partition(name);
            created := created + 1;
        END IF;
        m := (m + interval '1 month')::date;
    END LOOP;
    RETURN created;
END $$;

DO $$
DECLARE part record;
BEGIN
    FOR part IN
        SELECT c.relname FROM pg_inherits i
        JOIN pg_class c ON c.oid = i.inhrelid
        JOIN pg_class p ON p.oid = i.inhparent
        WHERE p.relname = 'views' AND p.relnamespace = current_schema()::regnamespace
    LOOP
        PERFORM protect_view_partition(part.relname);
    END LOOP;
END $$;

-- Обслуживание (удаление по сроку хранения из views_default) идёт без
-- контекста зрителя. Отдельные политики разрешают ему только найти и удалить
-- строки (DELETE с условием по столбцу требует и SELECT-политики) и только с
-- флагом plenka.maintenance; вставлять и менять так нельзя. Роль данных к
-- views_default не допущена вовсе (GrantApp), а у родителя views таких
-- политик нет.
CREATE POLICY retention_find ON views_default FOR SELECT USING (current_setting('plenka.maintenance', true) = 'on');
CREATE POLICY retention_delete ON views_default FOR DELETE USING (current_setting('plenka.maintenance', true) = 'on');

CREATE OR REPLACE FUNCTION drop_old_view_partitions(keep_months int) RETURNS int LANGUAGE plpgsql AS $$
DECLARE
    border date := (date_trunc('month', now()) - make_interval(months => keep_months))::date;
    part record;
    dropped int := 0;
BEGIN
    FOR part IN
        SELECT c.relname
        FROM pg_inherits i
        JOIN pg_class c ON c.oid = i.inhrelid
        JOIN pg_class p ON p.oid = i.inhparent
        WHERE p.relname = 'views' AND c.relname ~ '^views_[0-9]{4}_[0-9]{2}$'
    LOOP
        IF to_date(substr(part.relname, 7), 'YYYY_MM') < border THEN
            EXECUTE format('ALTER TABLE views DETACH PARTITION %I', part.relname);
            EXECUTE format('DROP TABLE %I', part.relname);
            dropped := dropped + 1;
        END IF;
    END LOOP;
    -- views_default теперь под RLS (как и все секции): удаление по сроку
    -- разрешено только с флагом обслуживания, политика retention ниже.
    PERFORM set_config('plenka.maintenance', 'on', true);
    DELETE FROM views_default WHERE started_at < border;
    PERFORM set_config('plenka.maintenance', '', true);
    RETURN dropped;
END $$;
