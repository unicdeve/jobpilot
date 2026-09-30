-- +goose Up
-- The companies whose hiring pages the candidate watches (ADR-0024 §2,
-- docs/02_features/17_company_watchlists.md). It references nothing and
-- nothing references it; "delete all data" is what deletes it.
CREATE TABLE watched_companies (
    id           TEXT PRIMARY KEY,
    provider     TEXT NOT NULL,
    region       TEXT NOT NULL DEFAULT '',
    board        TEXT NOT NULL,
    name         TEXT NOT NULL,
    added_from   TEXT NOT NULL,
    added_at     DATETIME NOT NULL,
    checked_at   DATETIME,
    check_status TEXT NOT NULL DEFAULT '',
    open_roles   INTEGER NOT NULL DEFAULT 0
);

-- A provider's board names are not case sensitive, so one board is watched
-- once however it was written.
CREATE UNIQUE INDEX idx_watched_companies_board
    ON watched_companies(provider, region, board COLLATE NOCASE);

-- +goose Down
DROP TABLE watched_companies;
