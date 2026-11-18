# Data

This folder is the database contract for the attendance server. The database is Postgres. The connection string is `DATABASE_URL`. The server opens it with `database/sql` and the driver `github.com/jackc/pgx/v5/stdlib`. The server does not use CGO. The server does not use SQLite. A file on the instance disk does not survive normal cloud deploys.

The schema is six tables: `students`, `lecturers`, `courses`, `roster`, `lectures`, and `attendance`. A nonce is a column on `attendance`. Freshness for sensor attendance is the lecturer signature plus `issued_at`, as defined in `../crypto/01-signatures.md` and `../http/01-routes.md`.

## Documents in this folder

- `CHANGELOG.md` records revisions of the documents in this folder.
- `01-schema.md` defines `students`, `lecturers`, `courses`, `roster`, `lectures`, and `attendance`.
