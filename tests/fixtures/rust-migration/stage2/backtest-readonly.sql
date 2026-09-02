PRAGMA journal_mode = DELETE;

CREATE TABLE jftrade_schema_meta (
    component_id TEXT PRIMARY KEY,
    version INTEGER NOT NULL,
    created_at TEXT NOT NULL
);

INSERT INTO jftrade_schema_meta (component_id, version, created_at)
VALUES ('backtest', 3, '2026-08-19T00:00:00Z');

CREATE TABLE local_klines__futu__us_aapl__1m__forward__r__a1b2c3d4 (
    end_time INTEGER NOT NULL,
    start_time INTEGER NOT NULL,
    open TEXT NOT NULL,
    high TEXT NOT NULL,
    low TEXT NOT NULL,
    close TEXT NOT NULL,
    volume TEXT NOT NULL,
    PRIMARY KEY (end_time)
) WITHOUT ROWID;

INSERT INTO local_klines__futu__us_aapl__1m__forward__r__a1b2c3d4
    (end_time, start_time, open, high, low, close, volume)
VALUES
    (1767294360000, 1767294300000, '100.25', '100.50000001', '100', '100.125', '1200.5'),
    (1767294300000, 1767294240000, '99.875', '100.25', '99.75', '100.25', '1000'),
    (1767294420000, 1767294360000, '100.125', '101', '100.00000001', '100.75', '980.125');
