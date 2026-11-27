-- ============ Тарифы ============
CREATE TABLE plans (
    id            text PRIMARY KEY,
    name          text NOT NULL,
    period_days   int NOT NULL CHECK (period_days > 0),
    price_kopecks bigint NOT NULL CHECK (price_kopecks > 0),
    active        boolean NOT NULL DEFAULT true
);
INSERT INTO plans VALUES
    ('month', 'Подписка на месяц', 30, 39900, true),
    ('year',  'Подписка на год',  365, 349900, true);

-- ============ Реплика каталога (из событий) ============
CREATE TABLE title_replica (
    id              uuid PRIMARY KEY,
    name            text NOT NULL,
    kind            text NOT NULL,
    status          text NOT NULL,
    in_subscription boolean NOT NULL,
    price_kopecks   bigint NOT NULL,
    revision        bigint NOT NULL
);

-- ============ Заказы — состояние саги оплаты ============
-- Заказ хранит шаг саги: после падения процесса оркестратор продолжит с
-- того места, где остановился (recovery-воркер).
--   charging  → failed                       (отказ банка)
--   charging  → completed                    (деньги списаны, доступ выдан)
--   charging  → refunding → refunded         (списали, но выдать доступ нельзя — компенсация)
CREATE TABLE orders (
    id             uuid PRIMARY KEY,
    user_id        uuid NOT NULL,
    kind           text NOT NULL CHECK (kind IN ('subscription', 'purchase')),
    plan_id        text REFERENCES plans (id),
    title_id       uuid,
    title_name     text NOT NULL DEFAULT '',
    amount_kopecks bigint NOT NULL CHECK (amount_kopecks > 0),
    status         text NOT NULL CHECK (status IN ('charging', 'failed', 'completed', 'refunding', 'refunded')),
    failure        text NOT NULL DEFAULT '',
    card_last4     text NOT NULL,
    idem_key       text NOT NULL,
    request_hash   text NOT NULL,
    created_at     timestamptz NOT NULL DEFAULT now(),
    updated_at     timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT orders_target CHECK ((kind = 'subscription' AND plan_id IS NOT NULL AND title_id IS NULL)
                                 OR (kind = 'purchase' AND title_id IS NOT NULL AND plan_id IS NULL)),
    CONSTRAINT orders_idem_uq UNIQUE (user_id, idem_key)
);
CREATE INDEX orders_user_idx ON orders (user_id, created_at DESC);
CREATE INDEX orders_stuck_idx ON orders (updated_at) WHERE status IN ('charging', 'refunding');
CREATE INDEX orders_admin_idx ON orders (created_at DESC, id DESC);

-- ============ Права доступа ============
CREATE TABLE purchases (
    user_id      uuid NOT NULL,
    title_id     uuid NOT NULL,
    order_id     uuid NOT NULL REFERENCES orders (id),
    purchased_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, title_id)   -- один фильм нельзя купить дважды
);

CREATE TABLE subscriptions (
    id                   uuid PRIMARY KEY,
    user_id              uuid NOT NULL,
    plan_id              text NOT NULL REFERENCES plans (id),
    order_id             uuid NOT NULL REFERENCES orders (id),
    status               text NOT NULL CHECK (status IN ('active', 'expired')),
    started_at           timestamptz NOT NULL,
    current_period_end   timestamptz NOT NULL,
    cancel_at_period_end boolean NOT NULL DEFAULT false
);
-- Не больше одной активной подписки на зрителя.
CREATE UNIQUE INDEX subscriptions_one_active ON subscriptions (user_id) WHERE status = 'active';

-- ============ Имитация платёжного шлюза ============
-- Внешняя система, которую мы «не контролируем». Живёт в той же базе только
-- ради учебной сборки; billing обращается к ней через порт Provider.
CREATE TABLE paysim_charges (
    ref            text PRIMARY KEY,   -- = id заказа: повторный Charge идемпотентен
    amount_kopecks bigint NOT NULL,
    card_last4     text NOT NULL,
    status         text NOT NULL CHECK (status IN ('succeeded', 'declined', 'refunded')),
    decline_reason text NOT NULL DEFAULT '',
    created_at     timestamptz NOT NULL DEFAULT now(),
    refunded_at    timestamptz
);

CREATE TABLE inbox (
    event_id     uuid PRIMARY KEY,
    event_type   text NOT NULL,
    handled_at   timestamptz NOT NULL DEFAULT now()
);