# ADR-0021: How data is deleted

## Status

Accepted.

This is decision D1 of `docs/02_features/15_data_deletion.md`, where the user chose option B: core decides what a deletion reaches, the adapter executes it, and the schema agrees.
This ADR says what each of those means, and how the three are kept from drifting apart.

It changes no accepted decision.
It implements the "Support deletion of user-owned data" principle of `docs/01_architecture/04_database.md`, and it changes one promise in `docs/02_features/12_agent_activity.md` (§4).

## Context

Almost nothing can be deleted today, and the reason is the schema rather than missing code.

Foreign keys are enforced on every connection (`docs/01_architecture/04_database.md`), and twelve of the eighteen foreign keys have no `ON DELETE` rule (read from `PRAGMA foreign_key_list`).
A row that anything references therefore cannot be deleted at all:

- a resume that an application used — measured: the daemon answers 500 with `FOREIGN KEY constraint failed`, and the desktop's Resumes screen shows it;
- a job, which its match and applications reference;
- an application, which its questions, cover letters, state history and browser runs reference;
- the candidate, which resumes, matches and applications reference.

The one place that deletes children first, `CandidateRepository.Delete`, handles the four candidate detail tables, which already cascade, and stops there.

Two facts about SQLite shape every option.

A foreign key's `ON DELETE` rule cannot be altered.
Changing one means rebuilding the table: create it anew under another name, copy the rows, drop the old table, rename the new one, recreate its indexes.

And SQLite's documented procedure for that rebuild requires `PRAGMA foreign_keys = OFF` while it runs, because dropping a table that other tables reference would otherwise fail or cascade.
That pragma has no effect inside a transaction.
Goose runs every migration in one by default, and every pooled connection turns foreign keys on in its DSN.

## Decision drivers

- AC-G-022: deletion handles derived data consistently.
  The database must neither keep an orphan nor refuse a deletion the product allows.
- The spec's principle 2: the user is told what goes before it goes, by kind and count, so what a deletion reaches must be known in advance and must be exactly what then happens.
- `AGENTS.md` principles 7 and 14: a deterministic rule lives in core and is testable without SQLite.
- The same rules serve the CLI and the desktop through the daemon (ADR-0012).
- A case the rules miss should fail a test, not leave data behind.

## Options

**A. Schema cascades only.**
Rebuild the foreign keys and let SQLite follow them.
Atomic and short, but what a deletion reaches is then written only in SQL: core cannot preview it, and nothing tests it except by deleting.

**B. Core declares the rules; the schema implements them; a test proves they agree.**
The chosen option, described below.

**C. Core decides and the adapter deletes in order; the schema is left alone.**
No table rebuild, but the foreign keys keep refusing deletions core allows, the adapter's delete order becomes the only thing between a rule and an orphan, and a table added later with no rule silently blocks deletion again.

## Decision

### 1. The rules are domain data

`internal/core/domain/deletion.go` declares, for every relationship between kinds of record, what deleting the referenced record does to the referencing one:

| Referencing | Referenced | Effect |
| --- | --- | --- |
| candidate details (experiences, skills, links, work authorizations) | candidate | delete |
| resume | candidate | delete |
| match | job, candidate | delete |
| application | job, candidate | delete |
| application | resume | detach |
| application's current cover letter | cover letter | detach |
| question and answer, cover letter, state transition | application | delete |
| agent run | application | delete |
| agent run | scout | detach |
| agent step | agent run | delete |
| scout | saved search | delete |

The kinds are domain words — job, application, resume — not table names.
The effects are the spec's decisions: an application's browser runs go with it (D4), and a resume is detached from the applications that used it (D5).

This table is the single statement of what a deletion reaches.
The preview is computed from it, the schema is checked against it (§3), and a change to it is a change to this ADR's table.

### 2. `DeletionService` in core owns what is allowed

`internal/core/services.DeletionService` handles every deletion the spec lists.

It decides what is refused, which the schema cannot express.
Today that is one rule: a cover letter version cannot be deleted while it is its application's current one.

It returns a preview for any deletion: the counts, by kind, of everything the rules reach, plus the facts a confirmation has to state, such as how many of the reached applications were submitted, or how many jobs marked not interested "clear jobs" keeps.
The counting is done through a port, `ports.DeletionStore`, which walks the rules in §1 over stored data.

### 3. The schema implements the rules, and a test proves it

One migration, `00015`, rebuilds the eight tables whose foreign keys have no rule, so that each foreign key's `ON DELETE` is the §1 effect: `CASCADE` for delete, `SET NULL` for detach.

A test in the SQLite adapter reads every foreign key from `PRAGMA foreign_key_list` and compares it, rule for rule, with the §1 table.
A foreign key with no rule, a rule the table does not declare, or a table added later without either fails it.
That test is what "the schema agrees" means: the preview in §2 and the deletion SQLite performs are computed from the same rules, and the test fails the moment they are not.

With that guarantee, deleting a single record is deleting its row.
The adapter does not reimplement the cascade, because a second implementation is a second thing that can be wrong.

### 4. What a deletion does, end to end

1. The client asks for the preview; the daemon returns the reach and a token.
   The token is a hash of the identities of every reached record.
2. The user confirms, having seen the reach.
3. The client sends the deletion with the token.
4. In one transaction, the adapter recomputes the reach, refuses the deletion with 409 if its token differs, and otherwise deletes.

The token exists because the confirmation is the user's consent, and consent covers what was shown.
A scout run between the preview and the confirmation can create an application for the job being deleted; without the token that application would be deleted unannounced.
It holds no state on the daemon, so it cannot expire or leak.

The bulk operations are the same shape.
"Clear activity" deletes every agent run.
"Clear jobs" deletes every job that no application uses and that is not marked not interested (spec D10).
Their reach is a query rather than one record, and the token covers it the same way.

Because the browser run of an application now goes with it, `docs/02_features/12_agent_activity.md`'s "deleting the scout or the application keeps the run and clears the link" becomes true for a scout only.
That spec is changed with the implementation.

### 5. How the migration runs

`00015` is a Go migration registered with `RunDB`, so goose does not wrap it in a transaction, and it runs SQLite's documented procedure on one connection it holds throughout:

1. Take a single connection from the pool.
2. `PRAGMA foreign_keys = OFF`.
3. Begin a transaction.
4. For each of the eight tables, parents before children: create the new table, copy every row column by column, drop the old table, rename the new one, recreate its indexes.
5. Run `PRAGMA foreign_key_check`; any row it reports rolls the whole migration back.
6. Commit, then `PRAGMA foreign_keys = ON` before the connection returns to the pool.

Step 6 matters as much as the rest.
A connection returned to the pool with foreign keys off would enforce nothing for as long as it lived, which is the defect `04_database.md` records fixing once already.

Every foreign key column on the referencing side gets an index, which most do not have today.
SQLite finds the rows a cascade reaches by scanning the referencing table without one, so a cascade over an unindexed column slows as history grows.

The rebuilt tables keep every column, type, default, `NOT NULL` and key exactly.
The migration's test asserts that `PRAGMA table_info` is identical before and after for all eight, that every row survives, and that the rules are the §1 table.
The table definitions are written out in the migration rather than generated from the live schema, so a reviewer reads the schema that results.

It has no down migration, as `00014` has none.
Undoing it would restore foreign keys that make deletion impossible.

"Not interested" is a separate, plain SQL migration, `00016`, adding `jobs.not_interested_at` and its index.
`jobs` references nothing, so it needs no rebuild.

### 6. Deleted means gone from disk

Spec D8, which this ADR makes concrete:

- Every connection sets `secure_delete` on, through the DSN like the other pragmas, so a deleted row's content is overwritten in the database file rather than left in a free page.
- After each deletion commits, the adapter runs `PRAGMA wal_checkpoint(TRUNCATE)`, so the deleted content's old pages do not stay readable in the WAL file.
- "Delete all data" also runs `VACUUM`, which rewrites the file without its free pages.

### 7. Delete all data

Spec D7: the installation is left as a fresh install is, apart from the device's theme choice.
It spans three stores that share no transaction, so it is ordered to be safe to repeat:

1. The browser worker's Chrome is closed, and `browser-profile/` is removed.
2. Each API key named in `credential_names` is deleted from the keychain, then its name.
   Keychain first, as writes already are (ADR-0019), so a failure leaves a name that a repeat finds and finishes rather than a key nothing lists.
3. Every row of every table is deleted in one transaction, the schema and its version table kept, then the database is vacuumed.
4. `daemon.log` is truncated, not removed, since the running daemon may hold it open.

A failure stops the sequence and says which step failed; running it again completes it, because every step is a no-op on what is already gone.
`daemon.json` is kept: it describes the running daemon, not the user.
A key supplied through a `JOBPILOT_CREDENTIAL_…` environment variable is not JobPilot's to delete, and the confirmation says so when one is set.

## Consequences

- Every deletion in the spec becomes possible, and the resume defect is fixed by the migration alone.
- What a deletion reaches is declared once, previewed from that declaration, and enforced by the schema, with a test that fails when the two differ.
- A future table that references another must declare its rule in §1, or the agreement test fails.
  That is the intended cost.
- `00015` rewrites eight tables once, on the first start after upgrading.
  It is transactional and all-or-nothing, but it takes time proportional to stored history.
- `secure_delete` makes every write that frees a page do extra work.
  A single-user local database does not feel it; it is noted in `04_database.md` with the other pragmas.
- A confirmation can be refused with 409 if something changed since it was shown.
  Both clients handle it by showing the new preview.

## Documentation changed with the implementation

- `docs/01_architecture/04_database.md`: the rule table, `secure_delete`, and the rebuild procedure for any later migration that changes a foreign key.
- `docs/02_features/12_agent_activity.md`: an application's browser runs go with it.
- `docs/02_features/15_data_deletion.md`: status, and the migration numbers.
- `docs/01_architecture/07_ipc.md`: preview routes, the token, and 409.
- `docs/00_product/current_state.md`.
