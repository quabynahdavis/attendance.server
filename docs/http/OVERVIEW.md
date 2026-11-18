# HTTP

This folder is the HTTP contract for the attendance server. The server speaks JSON over `net/http`. Clients register students, manage lecturers, courses, and rosters, open and end lectures, and submit attendance. There is no websocket and no session. There are no user accounts and no SSO.

Byte fields in JSON are standard base64, as produced by Go `encoding/base64.StdEncoding`, including padding. Decoded lengths are fixed: nonce 8 bytes, signatures 64 bytes, public keys 32 bytes, and lecturer, course, and lecture ids 16 bytes. The signed bytes those fields refer to are defined in `../crypto/01-signatures.md`. The stored rows are defined in `../data/01-schema.md`.

The platform terminates TLS. Clients use the HTTPS base URL in `ATTENDANCE_URL`. The Go process listens for HTTP on `PORT`. When `PORT` is unset, the port is 8080, for local development.

`ATTENDANCE_ADMIN_TOKEN` is the bearer token for creating lecturers, creating courses, and changing a roster. Lecturer mutations send `X-Lecturer-Id` and `X-Lecturer-Sig`. Student sensor attendance is authorized by the student signature in the body. Those rules are defined in `01-routes.md`.

## Documents in this folder

- `CHANGELOG.md` records revisions of the documents in this folder.
- `01-routes.md` lists each route, its JSON, and its status codes.
