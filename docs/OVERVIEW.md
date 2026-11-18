# Attendance server

This folder is the contract for a Go attendance server that is not implemented yet. Build to this contract. These documents are not a description of existing code.

The server is a cloud HTTPS service. The platform terminates TLS. The Go process listens for HTTP on `PORT`. When `PORT` is unset, the process listens on port 8080. Clients reach that port. There is no URL setting. There are no user accounts and no SSO. The server does not receive Bluetooth or audio. It checks only the JSON that a client submits.

## Runtime

The Go module root is `server/`. HTTP comes from the standard library `net/http`. The process listens on the TCP port named by `PORT`. When `PORT` is unset, the process listens on port 8080. Clients reach that port. There is no URL setting.

The database is a single SQLite file, as specified in `data/OVERVIEW.md`. The path comes from `ATTENDANCE_DB`. When `ATTENDANCE_DB` is unset, the file is `attendance.db` in the working directory. The server opens it with `database/sql` and the driver `modernc.org/sqlite`. The server does not use CGO. Foreign keys are enabled on the connection. On cloud, the SQLite file must live on a persistent disk. The container compose file mounts a volume for it. Routes are specified in `http/OVERVIEW.md`. Signatures are specified in `crypto/OVERVIEW.md`.

A later implementation should use this layout. Do not treat these paths as files that already exist:

- `cmd/attendance/main.go`
- `internal/httpapi`
- `internal/store`
- `internal/verify`

## Documents in this folder

- `CHANGELOG.md` records revisions of the documents in this folder.
- `http/` is the HTTP contract. Start at `http/OVERVIEW.md`.
- `crypto/` is the signature contract. Start at `crypto/OVERVIEW.md`.
- `data/` is the database contract. Start at `data/OVERVIEW.md`.
- `demo/` describes the demo seed. Start at `demo/OVERVIEW.md`.

## Out of scope

This contract is the Go server only. SSO, password accounts, zero-knowledge proofs, the Kotlin library, the Tauri app, and a specific cloud vendor are outside this contract.
