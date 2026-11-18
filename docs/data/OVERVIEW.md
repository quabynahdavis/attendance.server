# Data

This folder is the database contract for the attendance server. The database is a single SQLite file. The path comes from `ATTENDANCE_DB`. When `ATTENDANCE_DB` is unset, the path is `attendance.db` in the working directory. The server opens it with `database/sql` and the driver `modernc.org/sqlite`. The server does not use CGO. Foreign keys are enabled on the connection. The process is a cloud HTTPS service. The platform terminates TLS and sets `PORT`. On cloud, the SQLite file must live on a persistent disk. The container compose file mounts a volume for it.

The schema is six tables: `students`, `lecturers`, `courses`, `roster`, `lectures`, and `attendance`. A nonce is a column on `attendance`. Freshness for sensor attendance is the lecturer signature plus `issued_at`, as defined in `../crypto/01-signatures.md` and `../http/01-routes.md`.

## Documents in this folder

- `CHANGELOG.md` records revisions of the documents in this folder.
- `01-schema.md` defines `students`, `lecturers`, `courses`, `roster`, `lectures`, and `attendance`.
