# Changelog

## 2026-11-18

The route set was expanded. The server is a cloud HTTPS service. The database is SQLite. The path comes from `ATTENDANCE_DB`. The routes cover students, lecturers, courses, rosters, lectures, and attendance. The server has no base URL, only `PORT`.

The demo seed is documented in `demo/`. `make build` compiles `./cmd/attendance` to `./bin/attendance`. `make seed` loads `.env`, runs `go run ./cmd/seed`, and writes the SQLite file named by `ATTENDANCE_DB`.
