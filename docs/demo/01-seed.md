# Seed contents

`make seed` loads `.env` and runs `go run ./cmd/seed`. It writes the SQLite file named by `ATTENDANCE_DB`. When `ATTENDANCE_DB` is unset, the file is `attendance.db`. A second run does not insert duplicate rows.

Ed25519 keys are derived with SHA-256. Lecturer seeds are the strings `attendance-demo-lecturer-ama`, `attendance-demo-lecturer-kwesi`, and `attendance-demo-lecturer-lena`. Each student seed is the string `attendance-demo-student-` followed by that student's index number. The seed prints the public keys. It does not print private keys.

The demo week is 16–18 Nov 2026 UTC.

## Lecturers

Dr. Ama Boateng teaches CSCD 316 Operating Systems. Her lecturer id in hex is `11111111111111111111111111111111`. The same id in standard base64 is `EREREREREREREREREREREQ==`. Her key is SHA-256 of `attendance-demo-lecturer-ama`.

Prof. Kwesi Mensah teaches CSCD 402 Computer Security. His lecturer id in hex is `22222222222222222222222222222222`. The same id in standard base64 is `IiIiIiIiIiIiIiIiIiIiIg==`. His key is SHA-256 of `attendance-demo-lecturer-kwesi`.

Dr. Lena Owusu teaches DCIT 305 Information Systems. Her lecturer id in hex is `33333333333333333333333333333333`. The same id in standard base64 is `MzMzMzMzMzMzMzMzMzMzMw==`. Her key is SHA-256 of `attendance-demo-lecturer-lena`.

## Students

The seed writes these students. The index number is the student id used in the key string.

- 10928451 Efua Sarpong
- 10930118 Kofi Mensima
- 10887402 Abena Darko
- 10955290 Yaw Boateng
- 10866114 Nadia Hussein
- 10941833 Malik Adamu
- 10899021 Selorm Adjei
- 10977340 Ifeoma Okafor

## Rosters

CSCD 316 Operating Systems lists Efua Sarpong, Kofi Mensima, Abena Darko, Yaw Boateng, Nadia Hussein, and Malik Adamu.

CSCD 402 Computer Security lists Efua Sarpong, Abena Darko, Nadia Hussein, Selorm Adjei, and Ifeoma Okafor.

DCIT 305 Information Systems lists Kofi Mensima, Yaw Boateng, Selorm Adjei, and Malik Adamu.

## Lectures

CSCD 316 meets on Monday 16 Nov 2026 from 08:00 to 09:20 UTC. The lecture id in standard base64 is `sxYAAAAAAAAAAAAAAAAAAQ==`. The lecture is closed. Nadia Hussein, Abena Darko, Efua Sarpong, Kofi Mensima, and Yaw Boateng are present. Efua Sarpong is chirp high. Kofi Mensima is chirp low. Yaw Boateng is manual. Malik Adamu is absent.

CSCD 316 meets again on Wednesday 18 Nov 2026 at 09:00 UTC. The lecture id in standard base64 is `sxYAAAAAAAAAAAAAAAAAAg==`. The lecture is still open. It has no attendance rows. This is the live lecture.

CSCD 402 meets on Tuesday 17 Nov 2026 from 11:00 to 12:30 UTC. The lecture id in standard base64 is `tAIAAAAAAAAAAAAAAAAAAQ==`. The lecture is closed. Efua Sarpong and Nadia Hussein are chirp high. Selorm Adjei is chirp low. Abena Darko is manual. Ifeoma Okafor is absent.

DCIT 305 meets on Monday 16 Nov 2026 from 14:00 to 15:10 UTC. The lecture id in standard base64 is `swUAAAAAAAAAAAAAAAAAAQ==`. The lecture is closed. Kofi Mensima, Selorm Adjei, and Malik Adamu are chirp high. Yaw Boateng is chirp low.
