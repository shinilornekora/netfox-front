-- Права «по умолчанию» (pgkit.GrantApp) достаются и секциям, которые
-- обслуживание создаёт во время работы сервиса, — до следующего старта
-- роль данных могла бы обращаться к новой секции напрямую (RLS там уже
-- есть, но первый слой защиты не работал бы). Теперь protect_view_partition
-- сразу отзывает у новой секции все права, кроме прав владельца.
CREATE OR REPLACE FUNCTION protect_view_partition(part text) RETURNS void LANGUAGE plpgsql AS $$
DECLARE r record;
BEGIN
    EXECUTE format('ALTER TABLE %I ENABLE ROW LEVEL SECURITY', part);
    EXECUTE format('ALTER TABLE %I FORCE ROW LEVEL SECURITY', part);
    IF NOT EXISTS (SELECT 1 FROM pg_policies WHERE schemaname = current_schema()
                   AND tablename = part AND policyname = 'own_views') THEN
        EXECUTE format('CREATE POLICY own_views ON %I USING (user_id::text = current_viewer())
                        WITH CHECK (user_id::text = current_viewer())', part);
    END IF;
    FOR r IN
        SELECT DISTINCT a.grantee FROM pg_class c, aclexplode(c.relacl) a
        WHERE c.oid = to_regclass(quote_ident(part)) AND a.grantee <> c.relowner AND a.grantee <> 0
    LOOP
        EXECUTE format('REVOKE ALL ON %I FROM %I', part, r.grantee::regrole::text);
    END LOOP;
END $$;
