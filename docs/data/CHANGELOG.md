# Changelog

## 2026-11-18

The contract moved to cloud Postgres and the route set was expanded. The database is Postgres addressed by `DATABASE_URL`, opened with `database/sql` and `github.com/jackc/pgx/v5/stdlib`. The tables are `students`, `lecturers`, `courses`, `roster`, `lectures`, and `attendance`.
