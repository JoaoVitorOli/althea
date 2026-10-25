CREATE TABLE kv (
    namespace TEXT NOT NULL,
    key       TEXT NOT NULL,
    value     BLOB NOT NULL,
    PRIMARY KEY (namespace, key)
) WITHOUT ROWID;
