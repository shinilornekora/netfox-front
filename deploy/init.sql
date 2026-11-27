-- Одна база, схема на сервис (TRADEOFFS 2.1). У каждого сервиса две роли:
--   <svc>      — владелец своей схемы: миграции, обслуживание, ретранслятор
--                outbox. Чужие схемы не видит.
--   <svc>_app  — роль данных для запросов: только SELECT/INSERT/UPDATE/DELETE,
--                права выдаёт сервис при старте (pgkit.GrantApp). Не владеет
--                таблицами, поэтому не может снять RLS (TRADEOFFS 3.8).
-- Файл идемпотентен: compose выполняет его при каждом запуске (служба dbinit),
-- так что вторая роль появится и в уже существующей базе.
CREATE EXTENSION IF NOT EXISTS citext SCHEMA public;
REVOKE CREATE ON SCHEMA public FROM PUBLIC;

DO $$
DECLARE s text;
BEGIN
  FOREACH s IN ARRAY ARRAY['identity','catalog','billing','playback'] LOOP
    IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = s) THEN
      EXECUTE format('CREATE ROLE %I LOGIN PASSWORD %L', s, s || '_pw');
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = s || '_app') THEN
      EXECUTE format('CREATE ROLE %I LOGIN PASSWORD %L', s || '_app', s || '_app_pw');
    END IF;
    EXECUTE format('CREATE SCHEMA IF NOT EXISTS %I AUTHORIZATION %I', s, s);
    EXECUTE format('REVOKE ALL ON SCHEMA %I FROM PUBLIC', s);
    EXECUTE format('ALTER ROLE %I SET search_path = %I, public', s, s);
    EXECUTE format('ALTER ROLE %I SET search_path = %I, public', s || '_app', s);
  END LOOP;
END $$;
