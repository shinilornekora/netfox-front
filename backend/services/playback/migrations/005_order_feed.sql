-- Локальная проекция заказов billing (РХД ПР-5): playback узнаёт о
-- покупках, подписках и возвратах из события billing.orders.v1. По возврату
-- он сбрасывает кэш решений о доступе (5.3): возвращённый фильм перестаёт
-- открываться сразу, а не через 5 минут.
--
-- Отметка inbox и запись проекции — одна транзакция. Таблица закрыта RLS
-- под непривилегированной ролью сервиса: обработчик выставляет контекст
-- зрителя из события и физически не может записать строку другому зрителю.
CREATE TABLE order_feed (
    order_id   uuid PRIMARY KEY,
    user_id    uuid NOT NULL,
    kind       text NOT NULL,
    title_id   uuid,
    status     text NOT NULL,
    revision   bigint NOT NULL,
    changed_at timestamptz NOT NULL
);
CREATE INDEX order_feed_user_idx ON order_feed (user_id, changed_at DESC);
ALTER TABLE order_feed ENABLE ROW LEVEL SECURITY;
ALTER TABLE order_feed FORCE ROW LEVEL SECURITY;
CREATE POLICY own_orders ON order_feed USING (user_id::text = current_viewer()) WITH CHECK (user_id::text = current_viewer());
