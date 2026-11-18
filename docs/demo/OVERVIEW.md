# Demo seed

This folder describes the demo seed that loads a fixed week of courses, people, and lectures into the SQLite database. The seed is the `seed` target. It is separate from the HTTP contract in `../http/OVERVIEW.md` and the schema in `../data/OVERVIEW.md`.

`make build` compiles `./cmd/attendance` to `./bin/attendance`.

`make seed` loads `.env` and runs `go run ./cmd/seed`. It writes the SQLite file named by `ATTENDANCE_DB`. When `ATTENDANCE_DB` is unset, the file is `attendance.db`. Running the seed twice does not duplicate rows.

The seed prints public keys. It does not print private keys.

## Documents in this folder

- `CHANGELOG.md` records revisions of the documents in this folder.
- `01-seed.md` lists the lecturers, students, rosters, and lectures the seed writes.
