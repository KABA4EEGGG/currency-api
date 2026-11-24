-- Избранные валютные пары.
CREATE TABLE IF NOT EXISTS favorites (
    id         BIGSERIAL PRIMARY KEY,
    base       VARCHAR(3) NOT NULL,
    quote      VARCHAR(3) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT favorites_pair_unique UNIQUE (base, quote)
);
