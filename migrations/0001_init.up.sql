-- История конвертаций: основная таблица записи под нагрузочный тест.
CREATE TABLE IF NOT EXISTS conversions (
    id         BIGSERIAL PRIMARY KEY,
    base       VARCHAR(3) NOT NULL,
    quote      VARCHAR(3) NOT NULL,
    amount     NUMERIC(20, 6) NOT NULL,
    rate       NUMERIC(20, 10) NOT NULL,
    result     NUMERIC(20, 6) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Для агрегации /stats и сортировки истории.
CREATE INDEX IF NOT EXISTS conversions_pair_idx ON conversions (base, quote);
CREATE INDEX IF NOT EXISTS conversions_created_idx ON conversions (created_at DESC);
