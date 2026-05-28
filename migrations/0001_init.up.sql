CREATE TABLE users (
    id              bigserial PRIMARY KEY,
    username        text NOT NULL UNIQUE,
    email           text NOT NULL UNIQUE,
    password_hash   text NOT NULL,
    created_at      timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE channels (
    id               bigserial PRIMARY KEY,
    user_id          bigint NOT NULL UNIQUE REFERENCES users(id) ON DELETE CASCADE,
    name             text NOT NULL UNIQUE,
    title            text NOT NULL DEFAULT '',
    stream_key_hash  text NOT NULL,
    created_at       timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE sessions (
    id          text PRIMARY KEY,
    user_id     bigint NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    expires_at  timestamptz NOT NULL,
    created_at  timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX sessions_user_id_idx    ON sessions(user_id);
CREATE INDEX sessions_expires_at_idx ON sessions(expires_at);

CREATE TABLE stream_sessions (
    id          bigserial PRIMARY KEY,
    channel_id  bigint NOT NULL REFERENCES channels(id) ON DELETE CASCADE,
    started_at  timestamptz NOT NULL,
    ended_at    timestamptz,
    end_reason  text CHECK (end_reason IN ('normal', 'crashed', 'orphan_swept'))
);

CREATE INDEX stream_sessions_channel_id_idx ON stream_sessions(channel_id);
CREATE INDEX stream_sessions_open_idx       ON stream_sessions(channel_id) WHERE ended_at IS NULL;
