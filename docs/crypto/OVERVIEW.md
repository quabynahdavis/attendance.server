# Signatures

This folder is the signature contract for the attendance server. The algorithm is Ed25519, from the Go standard library `crypto/ed25519`. The server verifies a lecturer signature on lecture create, lecture end, and manual attendance. It verifies a lecturer signature and a student signature on sensor attendance. It does not receive Bluetooth or audio.

Public keys are 32 raw bytes. Signatures are 64 raw bytes. JSON and the lecturer headers carry those bytes as standard base64, as stated in `../http/01-routes.md`. The keys themselves are stored in the tables in `../data/01-schema.md`.

## Documents in this folder

- `CHANGELOG.md` records revisions of the documents in this folder.
- `01-signatures.md` defines each signed message and what those signatures do not cover.
