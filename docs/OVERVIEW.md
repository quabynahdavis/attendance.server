# Attendance server

This folder is the contract for a Go attendance server that is not implemented yet. Build to this contract. These documents are not a description of existing code.

The server is a cloud HTTPS service. Clients call the public base URL in `ATTENDANCE_URL`. The platform terminates TLS and sets `PORT`. The Go process listens for HTTP on `PORT`. When `PORT` is unset, the process listens on port 8080. That default is for local development. There are no user accounts and no SSO. The server does not receive Bluetooth or audio. It checks only the JSON that a client submits.

## Runtime

The Go module root is `server/`. HTTP comes from the standard library `net/http`. The process listens on the TCP port named by `PORT`. The public base URL is `ATTENDANCE_URL`.

The database is Postgres through the connection string `DATABASE_URL`, as specified in `data/OVERVIEW.md`. A file on the instance disk does not survive normal cloud deploys. Routes are specified in `http/OVERVIEW.md`. Signatures are specified in `crypto/OVERVIEW.md`.

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

## Out of scope

This contract is the Go server only. SSO, password accounts, zero-knowledge proofs, the Kotlin library, the Tauri app, and a specific cloud vendor are outside this contract.
