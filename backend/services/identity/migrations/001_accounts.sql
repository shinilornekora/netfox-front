CREATE EXTENSION IF NOT EXISTS citext;

CREATE TABLE accounts (
    id            uuid PRIMARY KEY,
    email         citext NOT NULL CONSTRAINT accounts_email_uq UNIQUE,
    password_hash text NOT NULL,
    display_name  text NOT NULL CHECK (char_length(display_name) BETWEEN 2 AND 60),
    birth_date    date CHECK (birth_date BETWEEN DATE '1900-01-01' AND CURRENT_DATE),
    role          text NOT NULL DEFAULT 'viewer' CHECK (role IN ('viewer', 'admin')),
    created_at    timestamptz NOT NULL DEFAULT now(),
    revision      bigint NOT NULL DEFAULT 1
);

-- Сессия — серверная запись. В cookie лежит случайный токен, в базе —
-- только его SHA-256: дамп базы не даёт войти в чужой аккаунт.
-- Срок скользящий (idle_expires_at продлевается при активности), но не
-- дольше absolute_expires_at от момента входа.
CREATE TABLE sessions (
    id                  uuid PRIMARY KEY,
    token_hash          bytea NOT NULL CONSTRAINT sessions_token_uq UNIQUE,
    account_id          uuid NOT NULL REFERENCES accounts (id) ON DELETE CASCADE,
    user_agent          text NOT NULL DEFAULT '',
    ip                  text NOT NULL DEFAULT '',
    created_at          timestamptz NOT NULL DEFAULT now(),
    last_seen_at        timestamptz NOT NULL DEFAULT now(),
    idle_expires_at     timestamptz NOT NULL,
    absolute_expires_at timestamptz NOT NULL,
    revoked_at          timestamptz
);
CREATE INDEX sessions_account_idx ON sessions (account_id) WHERE revoked_at IS NULL;
