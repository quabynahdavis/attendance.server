# Signature messages

Verification uses Ed25519 (`crypto/ed25519`). A signature is valid only when `ed25519.Verify` succeeds for the public key, the exact message bytes below, and the 64-byte signature.

Each message is raw bytes. It is not the JSON text and it is not the base64 text. Ids and nonces are copied in the order they are stored. They are not reversed and they are not encoded as decimal text. Integer fields are unsigned big-endian integers of the width stated for that message.

A label prefix is ASCII, then one newline byte, `0x0A`. The prefix has no length prefix and no trailing NUL. Routes and status codes are in `../http/01-routes.md`. A bad decoded length on a signature or an id is rejected by that route before verification. A verification failure is 401.

The server verifies a signature and does not store it.

## Lecturer headers

Lecture create, lecture end, and manual attendance authorize the lecturer with two headers:

- `X-Lecturer-Id` is standard base64 of the 16-byte lecturer id.
- `X-Lecturer-Sig` is standard base64 of the 64-byte signature.

Verify the signature with the 32-byte `public_key` stored for that lecturer id. The lecturer id must be the `lecturer_id` of the course for that route. For lecture create, that course is the course in the path. For lecture end and manual attendance, that course is the course of the lecture in the path.

## Lecture create

`POST /courses/{id}/lectures` signs this message. The layout is big-endian for the uint64. The total length is 39 bytes.

`lecture.create\n` || `courseId` (16) || `startsAt` (uint64)

| Offset | Length | Contents |
| --- | --- | --- |
| 0 | 15 | ASCII `lecture.create` and a newline byte |
| 15 | 16 | Raw course id |
| 31 | 8 | `startsAt` as a big-endian uint64 |

`startsAt` in this message is the same integer as the JSON `startsAt` field. The JSON number must be an integer from 0 through 18446744073709551615. The JSON number and these eight bytes are the same integer.

## Lecture end

`POST /lectures/{id}/end` signs this message. The total length is 28 bytes.

`lecture.end\n` || `lectureId` (16)

| Offset | Length | Contents |
| --- | --- | --- |
| 0 | 12 | ASCII `lecture.end` and a newline byte |
| 12 | 16 | Raw lecture id |

The request has no JSON body. The signed lecture id is the lecture named in the path.

## Manual attendance

`POST /lectures/{id}/attendance/manual` signs this message. The length depends on the index number.

`attendance.manual\n` || `lectureId` (16) || `indexNumber` as UTF-8

| Offset | Length | Contents |
| --- | --- | --- |
| 0 | 18 | ASCII `attendance.manual` and a newline byte |
| 18 | 16 | Raw lecture id |
| 34 | variable | The index number as UTF-8, with no length prefix and no trailing NUL |

The index number bytes are the UTF-8 encoding of the JSON `indexNumber` string.

## Sensor attendance, lecturer message

`POST /lectures/{id}/attendance` carries the lecturer signature in the JSON field `lecturerSig`. That route does not use `X-Lecturer-Id` or `X-Lecturer-Sig`.

The lecturer signs exactly 28 bytes. The layout is big-endian for the uint32:

`lectureId` (16) || `issuedAt` (uint32) || `nonce` (8)

| Offset | Length | Contents |
| --- | --- | --- |
| 0 | 16 | Raw lecture id |
| 16 | 4 | `issuedAt` as a big-endian uint32 |
| 20 | 8 | Raw nonce |

`issuedAt` in this message is the same uint32 as the JSON `issuedAt` field. The JSON number and these four bytes are the same integer.

Verify this signature with the 32-byte `public_key` of the lecturer who owns the course of that lecture. The check uses that stored key. It does not use a key supplied beside the signature.

## Sensor attendance, student message

The student signs exactly 24 bytes. The signature is the JSON field `studentSig`.

`lectureId` (16) || `nonce` (8)

| Offset | Length | Contents |
| --- | --- | --- |
| 0 | 16 | Raw lecture id |
| 16 | 8 | Raw nonce |

Verify this signature with the 32-byte `public_key` stored for that student's index number.

The student message does not include `issuedAt`, `rssi`, or `chirpOk`.

## What the signatures establish

The lecture-create signature binds the lecturer to that course id and that `startsAt`. The lecture-end signature binds the lecturer to that lecture id. The manual-attendance signature binds the lecturer to that lecture id and that index number.

On sensor attendance, the lecturer signature binds the lecture id, the `issuedAt` time, and the nonce. The student signature binds that student to the lecture id and the nonce. Together with the server clock check on `issuedAt`, this is the freshness rule for a sensor submission. The contract has no nonce table. See `../data/01-schema.md`.

`rssi` and `chirpOk` are client assertions. They are not proof of location. A valid pair of signatures does not show that the phone saw a signal, heard a chirp, or was in the room. The server stores the submitted `rssi` and `chirpOk` only after the HTTP checks in `../http/01-routes.md` accept the request.
