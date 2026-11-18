# Routes

Request and response bodies are JSON, except where a route below says the body is empty or the status is 204. Field names below are the JSON names. A body that is not JSON, a missing required field, or a field of the wrong JSON type is malformed and returns 400.

Standard base64 means Go `encoding/base64.StdEncoding`, including `+`, `/`, and `=` padding. The signed messages and the keys used to verify them are defined in `../crypto/01-signatures.md`. Tables and column names are defined in `../data/01-schema.md`.

Path segments that are ids are standard base64 of 16 bytes. Clients percent-encode those segments, so `+` is `%2B`, `/` is `%2F`, and `=` is `%3D`. The server decodes the segment to raw bytes before looking up the row. A segment that is not valid standard base64, or whose decoded length is not 16 bytes, returns 400. An `indexNumber` path segment is the text index number. It is not base64.

Read routes in this contract do not require a bearer token or a lecturer signature. `GET /health` and `GET /courses` are open. The other GET routes are open as well.

These are the only routes in the contract.

## Authorization

There are no user accounts and no SSO.

`ATTENDANCE_ADMIN_TOKEN` is a bearer token. Creating a lecturer, creating a course, and changing a roster require this header:

`Authorization: Bearer <token>`

Compare the presented token to `ATTENDANCE_ADMIN_TOKEN` in constant time. A missing header, a scheme other than `Bearer`, or a token that does not match returns 401. When `ATTENDANCE_ADMIN_TOKEN` is unset, no presented token matches, and those routes return 401. Check the token before the other checks on that route.

Lecture create, lecture end, and manual attendance are authorized by an Ed25519 signature from that course's lecturer. The client sends:

- `X-Lecturer-Id`: standard base64 of the 16-byte lecturer id.
- `X-Lecturer-Sig`: standard base64 of the 64-byte signature.

The signed bytes for each of those routes are stated on the route and laid out in `../crypto/01-signatures.md`. The lecturer id must be the lecturer of that course. A missing header, a value that is not valid standard base64, a decoded length other than 16 bytes for the id or 64 bytes for the signature, a lecturer id that is not the lecturer of that course, or a signature that does not verify returns 401.

Student sensor attendance is authorized by the student Ed25519 signature in the body, together with the lecturer signature in the body. That route does not use the lecturer headers.

## GET /health

Returns liveness. No auth.

Success returns 200 and this body:

```json
{"ok": true}
```

## POST /students

Claims an index number and stores that student's Ed25519 public key. Registration is open. This route does not require the admin token.

Request body:

- `indexNumber` (string, required).
- `publicKey` (string, required). Standard base64 that decodes to 32 bytes.
- `displayName` (string, optional). When the field is absent, store `display_name` as NULL.

A malformed body, a `publicKey` that is not valid standard base64, or a decoded key whose length is not 32 bytes returns 400.

The first writer owns that index number. The first request that stores a key returns 200. A later request with the same index number and the same 32 key bytes returns 200 and leaves the stored row unchanged, including `display_name`. A later request with the same index number and a different key returns 409. If two requests race for one new index number, the stored key wins, and a request whose key does not match that stored key returns 409.

The response body is empty. The status code is the result.

## GET /students/{indexNumber}

Returns one student. The path segment is the text index number.

A missing student returns 404.

Success returns 200 and this body:

```json
{
  "indexNumber": "<index number>",
  "publicKey": "<standard base64 of the 32-byte key>",
  "displayName": null
}
```

`displayName` is the stored `display_name`, or JSON null when that column is NULL.

## GET /students/{indexNumber}/attendance

Returns the attendance rows for one student.

A missing student returns 404. A student with no attendance rows returns 200 and an empty JSON array.

Each object has `lectureId`, `courseId`, `courseCode`, `issuedAt`, `chirpOk`, `source`, `confidence`, and `createdAt`. `lectureId` and `courseId` are standard base64 of the stored 16-byte ids. `courseId` and `courseCode` come from the lecture's course. `chirpOk` is the stored boolean. `source` is `sensor` or `manual`. `confidence` is `high` when `chirpOk` is true, and `low` when `chirpOk` is false. `confidence` is not a column. `issuedAt` and `createdAt` are the stored unix seconds.

Success returns 200 and a JSON array of those objects:

```json
[
  {
    "lectureId": "<standard base64 of the 16-byte lecture id>",
    "courseId": "<standard base64 of the 16-byte course id>",
    "courseCode": "<course code>",
    "issuedAt": 0,
    "chirpOk": false,
    "source": "sensor",
    "confidence": "low",
    "createdAt": 0
  }
]
```

## POST /lecturers

Creates one lecturer. Requires the admin token. The server draws 16 random bytes and stores them as the lecturer id.

Request body:

- `displayName` (string, required).
- `publicKey` (string, required). Standard base64 that decodes to 32 bytes.

A malformed body, or a `publicKey` that does not decode to 32 bytes, returns 400.

Success returns 200 and this body:

```json
{
  "id": "<standard base64 of the 16-byte lecturer id>"
}
```

## GET /lecturers/{id}

Returns one lecturer. The path segment is the lecturer id.

A missing lecturer returns 404.

Success returns 200 and this body:

```json
{
  "id": "<standard base64 of the 16-byte id>",
  "publicKey": "<standard base64 of the 32-byte key>",
  "displayName": "<display name>"
}
```

## GET /lecturers/{id}/courses

Returns the courses taught by one lecturer.

A missing lecturer returns 404. A lecturer with no courses returns 200 and an empty JSON array.

Each object has `id`, `code`, and `title`. `id` is standard base64 of the 16-byte course id.

Success returns 200 and a JSON array of those objects:

```json
[
  {
    "id": "<standard base64 of the 16-byte course id>",
    "code": "<code>",
    "title": "<title>"
  }
]
```

## POST /courses

Creates one course. Requires the admin token. The server draws 16 random bytes and stores them as the course id.

Request body:

- `code` (string, required).
- `title` (string, required).
- `lecturerId` (string, required). Standard base64 that decodes to 16 bytes.

Run these checks in order and stop at the first failure:

1. A malformed body, or a `lecturerId` that does not decode to 16 bytes, returns 400.
2. An unknown lecturer returns 404.
3. A `code` that is already stored on another course returns 409.
4. Insert the course and return 200 with this body:

```json
{
  "id": "<standard base64 of the 16-byte course id>"
}
```

`code` matches an existing course by exact string equality.

## GET /courses

Returns every course. No auth.

A server with no courses returns 200 and an empty JSON array.

Each object has `id`, `code`, `title`, and `lecturerId`. `id` and `lecturerId` are standard base64 of the stored 16-byte ids.

Success returns 200 and a JSON array of those objects:

```json
[
  {
    "id": "<standard base64 of the 16-byte course id>",
    "code": "<code>",
    "title": "<title>",
    "lecturerId": "<standard base64 of the 16-byte lecturer id>"
  }
]
```

## GET /courses/{id}

Returns one course. The path segment is the course id.

A missing course returns 404.

Success returns 200 and this body:

```json
{
  "id": "<standard base64 of the 16-byte id>",
  "code": "<code>",
  "title": "<title>",
  "lecturerId": "<standard base64 of the 16-byte lecturer id>"
}
```

## POST /courses/{id}/roster

Adds one student to a course roster. Requires the admin token.

Request body:

- `indexNumber` (string, required).

Run these checks in order and stop at the first failure:

1. A bad course id in the path returns 400.
2. A malformed body returns 400.
3. An unknown course returns 404.
4. An unknown student returns 404.
5. A student who is already on the roster returns 200 and an empty body. The roster row stays as it is.
6. Insert the roster row. Return 200 and an empty body.

## DELETE /courses/{id}/roster/{indexNumber}

Removes one student from a course roster. Requires the admin token. The course segment is the course id. The index-number segment is the text index number.

A bad course id in the path returns 400. An unknown course returns 404. When the course exists, return 204 whether or not that student was on the roster. A 204 response has no body.

## GET /courses/{id}/roster

Returns the roster of one course.

A missing course returns 404. A course with no roster rows returns 200 and an empty JSON array.

Each object has `indexNumber` and `displayName`. `displayName` is the student's `display_name`, or JSON null when that column is NULL.

Success returns 200 and a JSON array of those objects:

```json
[
  {
    "indexNumber": "<index number>",
    "displayName": null
  }
]
```

## POST /courses/{id}/lectures

Opens one lecture for a course. Requires a lecturer signature. The server draws 16 random bytes and stores them as the lecture id. `ends_at` is NULL.

Request body:

- `startsAt` (integer, required). Unix seconds.

Signed bytes: ASCII `lecture.create\n` || `courseId` (16) || `startsAt` (uint64 big-endian). The byte layout is in `../crypto/01-signatures.md`.

Run these checks in order and stop at the first failure:

1. A bad course id in the path returns 400.
2. A malformed body, or a `startsAt` outside the uint64 range 0 through 18446744073709551615, returns 400.
3. An unknown course returns 404.
4. A wrong lecturer id or a signature that does not verify returns 401.
5. Insert the lecture with `ends_at` NULL. Return 200 and this body:

```json
{
  "id": "<standard base64 of the 16-byte lecture id>"
}
```

## GET /courses/{id}/lectures

Returns the lectures of one course.

A missing course returns 404. A course with no lectures returns 200 and an empty JSON array.

Each object has `id`, `startsAt`, and `endsAt`. `id` is standard base64 of the 16-byte lecture id. `startsAt` is the stored unix seconds. `endsAt` is the stored unix seconds, or JSON null when the lecture is still open.

Success returns 200 and a JSON array of those objects:

```json
[
  {
    "id": "<standard base64 of the 16-byte lecture id>",
    "startsAt": 0,
    "endsAt": null
  }
]
```

## GET /lectures/{id}

Returns one lecture. The path segment is the lecture id.

A missing lecture returns 404.

Success returns 200 and this body:

```json
{
  "id": "<standard base64 of the 16-byte id>",
  "courseId": "<standard base64 of the 16-byte course id>",
  "startsAt": 0,
  "endsAt": null
}
```

`endsAt` is JSON null when `ends_at` is NULL.

## POST /lectures/{id}/end

Ends one lecture. Requires a lecturer signature. The request has no body. The server sets `ends_at` to the server time in unix seconds.

Signed bytes: ASCII `lecture.end\n` || `lectureId` (16). The byte layout is in `../crypto/01-signatures.md`.

Run these checks in order and stop at the first failure:

1. A bad lecture id in the path returns 400.
2. An unknown lecture returns 404.
3. A wrong lecturer id or a signature that does not verify returns 401. The lecturer id must be the lecturer of that lecture's course.
4. A lecture that has already ended returns 409.
5. Set `ends_at` to the server time. Return 200 and the lecture object, in the same shape as `GET /lectures/{id}`.

## POST /lectures/{id}/attendance

Submits one student's sensor attendance for one lecture. The server checks the submitted JSON and the two signatures. It does not receive Bluetooth or audio.

The lecture id is the path segment. The body does not carry a separate lecture id.

Request body:

- `indexNumber` (string, required).
- `nonce` (string, required). Standard base64 that decodes to 8 bytes.
- `issuedAt` (integer, required). Unix seconds. The value must fit in an unsigned 32-bit integer, from 0 through 4294967295, because the lecturer message stores it as a uint32.
- `lecturerSig` (string, required). Standard base64 that decodes to 64 bytes.
- `studentSig` (string, required). Standard base64 that decodes to 64 bytes.
- `rssi` (integer, required). Stored and returned as submitted on the first insert.
- `chirpOk` (boolean, required).

Lecturer signed bytes, 28 bytes: `lectureId` (16) || `issuedAt` (uint32 big-endian) || `nonce` (8).

Student signed bytes, 24 bytes: `lectureId` (16) || `nonce` (8).

The student bytes do not include `rssi` or `chirpOk`. Those two fields are client assertions. They are not proof of location. See `../crypto/01-signatures.md`.

Run the following checks in order and stop at the first failure:

1. A malformed body, a bad lecture id in the path, invalid standard base64, or a decoded length other than the lengths above, including an `issuedAt` outside the uint32 range, returns 400.
2. An unknown student returns 404.
3. An unknown lecture returns 404.
4. A lecture that has already ended returns 409. Server time more than 300 seconds before `startsAt` returns 409. The comparison uses the server clock in unix seconds. A server time equal to `startsAt` minus 300 is accepted. A later server time is accepted while `ends_at` is NULL.
5. A student who is not on that course's roster returns 403. The course is the lecture's course.
6. A student signature that does not verify under the student's public key returns 401.
7. A lecturer signature that does not verify under the course lecturer's public key returns 401.
8. An `issuedAt` more than 20 seconds from server time returns 409. The absolute difference is compared in unix seconds. A difference of 20 seconds still passes.
9. `ATTENDANCE_RSSI_MIN` is optional. When it is unset, `rssi` rejects nobody. When it is set, it is an integer cutoff. Return 409, and do not write a row, only when `chirpOk` is false and `rssi` is below that cutoff. A request with `chirpOk` true is accepted at any `rssi`. An `rssi` equal to the cutoff passes.
10. Insert the attendance row with `source` `sensor`. Set `created_at` to the server time in unix seconds. On a primary-key conflict for `(index_number, lecture_id)`, do not insert a second row. When the stored `chirp_ok` is false, the submitted `chirpOk` is true, and the signatures on this request verified in checks 6 and 7, set `chirp_ok` to true and set `source` to `sensor`. Leave `nonce`, `issued_at`, `rssi`, and `created_at` as they were first stored. Do not change `chirp_ok` from true to false. A stored manual row stays a manual row unless this upgrade applies. Return the stored row, after that update, with 200.

The first insert and the conflict path both return 200. The success body is the stored row after any allowed update:

```json
{
  "indexNumber": "<index number>",
  "lectureId": "<standard base64 of the stored 16-byte lecture id>",
  "nonce": "<standard base64 of the stored 8-byte nonce>",
  "issuedAt": 0,
  "rssi": 0,
  "chirpOk": false,
  "source": "sensor",
  "createdAt": 0,
  "confidence": "low"
}
```

`confidence` is `high` when the returned `chirpOk` is true, and `low` when it is false. `confidence` is not a column. On a conflict, `lectureId`, `nonce`, `issuedAt`, `rssi`, and `createdAt` come from the stored row. `chirpOk`, `source`, and `confidence` reflect the stored row after the upgrade rule.

There is no nonce table. A repeated submission for the same student and lecture follows the conflict rule in check 10. Freshness is the lecturer signature together with the `issuedAt` check in check 8.

## POST /lectures/{id}/attendance/manual

Records attendance for one roster student without a sensor submission. Requires a lecturer signature.

Request body:

- `indexNumber` (string, required).
- `note` (optional). The server does not store `note`.

Signed bytes: ASCII `attendance.manual\n` || `lectureId` (16) || `indexNumber` as UTF-8. The byte layout is in `../crypto/01-signatures.md`.

Run these checks in order and stop at the first failure:

1. A bad lecture id in the path, or a malformed body, returns 400.
2. An unknown lecture returns 404.
3. A wrong lecturer id or a signature that does not verify returns 401. The lecturer id must be the lecturer of that lecture's course.
4. A lecture that has already ended returns 409.
5. A student who is not on that course's roster returns 403.
6. When an attendance row already exists for that student and lecture, return that row with 200 and do not change it. A sensor row stays a sensor row.
7. Insert a row with `source` `manual`, `chirp_ok` false, `rssi` 0, a nonce of 8 zero bytes, `issued_at` set to the server time in unix seconds, and `created_at` set to that same server time. Return 200 and the stored row.

The success body uses the same fields as `POST /lectures/{id}/attendance`: `indexNumber`, `lectureId`, `nonce`, `issuedAt`, `rssi`, `chirpOk`, `source`, `createdAt`, and `confidence`. On a new manual row, `chirpOk` is false, `source` is `manual`, `rssi` is 0, and `confidence` is `low`.

## GET /lectures/{id}/attendance

Returns the attendance list for one lecture.

A missing lecture returns 404. A lecture with no attendance rows returns 200 with `count` 0 and an empty `rows` array.

Success returns 200 and this body:

```json
{
  "count": 1,
  "rows": [
    {
      "indexNumber": "<index number>",
      "displayName": "<students.display_name, or null>",
      "rssi": 0,
      "chirpOk": false,
      "source": "sensor",
      "confidence": "low",
      "createdAt": 0
    }
  ]
}
```

`count` is the number of objects in `rows`. There is one object for each attendance row of that lecture. `displayName` is the `display_name` of the matching student, or JSON null when that column is NULL. `confidence` is `high` when `chirpOk` is true and `low` when `chirpOk` is false. `rssi` and `createdAt` are the stored values. `source` is the stored `sensor` or `manual`.

The list does not include the nonce, the signatures, or `issuedAt`.

## GET /courses/{id}/attendance/summary

Returns one attendance summary for a course.

A missing course returns 404.

`lecturesHeld` is the number of lectures of that course that have ended, plus each lecture of that course that is still open. A course with no lectures has `lecturesHeld` 0.

The `students` array has one object per roster row. Each object has `indexNumber`, `displayName`, `attended`, and `percent`. `displayName` is the student's `display_name`, or JSON null when that column is NULL. `attended` is the number of that course's lectures for which this student has an attendance row. `percent` is `attended` divided by `lecturesHeld`. When `lecturesHeld` is 0, `percent` is 0.

A course with an empty roster returns 200 with `students` as an empty array.

Success returns 200 and this body:

```json
{
  "lecturesHeld": 0,
  "students": [
    {
      "indexNumber": "<index number>",
      "displayName": null,
      "attended": 0,
      "percent": 0
    }
  ]
}
```
