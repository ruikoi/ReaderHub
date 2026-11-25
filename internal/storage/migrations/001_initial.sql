CREATE TABLE IF NOT EXISTS articles (
    id TEXT PRIMARY KEY,
    fingerprint TEXT UNIQUE NOT NULL,
    metadata TEXT NOT NULL,
    search_text TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS notes (
    id TEXT PRIMARY KEY,
    article_id TEXT NOT NULL REFERENCES articles(id) ON DELETE CASCADE,
    quote TEXT NOT NULL,
    text TEXT NOT NULL,
    created_at TEXT NOT NULL
);
