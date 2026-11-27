-- Срок хранения истории просмотров. Старый месяц убирается целиком:
-- DETACH PARTITION + DROP TABLE — это операции над метаданными, они не
-- трогают строки и не раздувают WAL, в отличие от DELETE по миллионам строк.
-- Удаляются только месячные секции views_YYYY_MM старше keep_months полных
-- месяцев; секция DEFAULT не трогается.
CREATE FUNCTION drop_old_view_partitions(keep_months int) RETURNS int LANGUAGE plpgsql AS $$
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
    RETURN dropped;
END $$;
