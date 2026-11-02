-- Translations kept, to answer again at once and without the network.
CREATE TABLE translator_cache (
    key         TEXT    PRIMARY KEY,    -- sha256 of the provider, the languages and the text
    translation TEXT    NOT NULL,
    used        INTEGER NOT NULL        -- Unix milliseconds of the last use
);

CREATE INDEX translator_cache_used ON translator_cache (used DESC);
