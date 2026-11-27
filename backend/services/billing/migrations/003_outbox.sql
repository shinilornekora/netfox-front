-- Transactional outbox billing (РХД ПР-4): событие об изменении заказа
-- записывается той же транзакцией, что и сам заказ и ответ команды
-- (заказ с ключом идемпотентности и есть сохранённый ответ checkout).
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

-- Изоляция outbox политиками RLS. Бизнес-транзакции billing могут только
-- ДОБАВЛЯТЬ события; читать, брать в аренду, отмечать отправленными и
-- удалять может только ретранслятор, который явно объявляет себя
-- (set_config('plenka.relay','on', true) в своей транзакции). Ошибка или
-- инъекция в коде оплаты не может прочитать очередь чужих событий или
-- «потерять» событие, пометив его отправленным.
ALTER TABLE outbox ENABLE ROW LEVEL SECURITY;
ALTER TABLE outbox FORCE ROW LEVEL SECURITY;
CREATE POLICY outbox_append ON outbox FOR INSERT WITH CHECK (true);
CREATE POLICY outbox_relay_read ON outbox FOR SELECT
    USING (current_setting('plenka.relay', true) = 'on');
CREATE POLICY outbox_relay_update ON outbox FOR UPDATE
    USING (current_setting('plenka.relay', true) = 'on')
    WITH CHECK (current_setting('plenka.relay', true) = 'on');
CREATE POLICY outbox_relay_delete ON outbox FOR DELETE
    USING (current_setting('plenka.relay', true) = 'on');
