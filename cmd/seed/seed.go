package main

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"attendance/internal/store"
)

const (
	idAma       = "11111111111111111111111111111111"
	idKwesi     = "22222222222222222222222222222222"
	idLena      = "33333333333333333333333333333333"
	idCSCD316   = "a3160000000000000000000000000001"
	idCSCD402   = "a4020000000000000000000000000001"
	idDCIT305   = "a3050000000000000000000000000001"
	idLec316Mon = "b3160000000000000000000000000001"
	idLec316Wed = "b3160000000000000000000000000002"
	idLec402Tue = "b4020000000000000000000000000001"
	idLec305Mon = "b3050000000000000000000000000001"
)

type demoStudent struct {
	index string
	name  string
}

type sensorMark struct {
	index string
	chirp bool
	rssi  int
}

var demoStudents = []demoStudent{
	{"10928451", "Efua Sarpong"},
	{"10930118", "Kofi Mensima"},
	{"10887402", "Abena Darko"},
	{"10955290", "Yaw Boateng"},
	{"10866114", "Nadia Hussein"},
	{"10941833", "Malik Adamu"},
	{"10899021", "Selorm Adjei"},
	{"10977340", "Ifeoma Okafor"},
}

func seed(ctx context.Context, db *store.Store, path string) error {
	for _, student := range demoStudents {
		name := student.name
		pub := publicKey("attendance-demo-student-" + student.index)
		if _, err := db.RegisterStudent(ctx, student.index, pub, &name); err != nil {
			return err
		}
	}

	raw, err := sql.Open("sqlite", path+"?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)")
	if err != nil {
		return err
	}
	defer raw.Close()

	mon316 := time.Date(2026, 11, 16, 8, 0, 0, 0, time.UTC)
	mon316End := time.Date(2026, 11, 16, 9, 20, 0, 0, time.UTC)
	wed316 := time.Date(2026, 11, 18, 9, 0, 0, 0, time.UTC)
	tue402 := time.Date(2026, 11, 17, 11, 0, 0, 0, time.UTC)
	tue402End := time.Date(2026, 11, 17, 12, 30, 0, 0, time.UTC)
	mon305 := time.Date(2026, 11, 16, 14, 0, 0, 0, time.UTC)
	mon305End := time.Date(2026, 11, 16, 15, 10, 0, 0, time.UTC)

	if err := seedLecturer(ctx, db, raw, idAma, "Dr. Ama Boateng", "attendance-demo-lecturer-ama", idCSCD316, "CSCD 316", "Operating Systems",
		[]string{"10928451", "10930118", "10887402", "10955290", "10866114", "10941833"},
		[]lectureSpec{
			{
				id: idLec316Mon, start: mon316, end: &mon316End,
				sensor: []sensorMark{
					{"10928451", true, -48},
					{"10930118", false, -67},
					{"10887402", true, -55},
					{"10866114", true, -52},
				},
				manual: []string{"10955290"},
			},
			{id: idLec316Wed, start: wed316},
		}); err != nil {
		return err
	}

	if err := seedLecturer(ctx, db, raw, idKwesi, "Prof. Kwesi Mensah", "attendance-demo-lecturer-kwesi", idCSCD402, "CSCD 402", "Computer Security",
		[]string{"10928451", "10887402", "10866114", "10899021", "10977340"},
		[]lectureSpec{
			{
				id: idLec402Tue, start: tue402, end: &tue402End,
				sensor: []sensorMark{
					{"10928451", true, -50},
					{"10866114", true, -58},
					{"10899021", false, -71},
				},
				manual: []string{"10887402"},
			},
		}); err != nil {
		return err
	}

	return seedLecturer(ctx, db, raw, idLena, "Dr. Lena Owusu", "attendance-demo-lecturer-lena", idDCIT305, "DCIT 305", "Information Systems",
		[]string{"10930118", "10955290", "10899021", "10941833"},
		[]lectureSpec{
			{
				id: idLec305Mon, start: mon305, end: &mon305End,
				sensor: []sensorMark{
					{"10930118", true, -46},
					{"10955290", false, -64},
					{"10899021", true, -53},
					{"10941833", true, -60},
				},
			},
		})
}

type lectureSpec struct {
	id     string
	start  time.Time
	end    *time.Time
	sensor []sensorMark
	manual []string
}

func seedLecturer(ctx context.Context, db *store.Store, raw *sql.DB, lecturerHex, name, label, courseHex, code, title string, roster []string, lectures []lectureSpec) error {
	lecturerID := mustID(lecturerHex)
	_, err := db.GetLecturer(ctx, lecturerID)
	if err == nil {
		return nil
	}
	if !errors.Is(err, store.ErrNotFound) {
		return err
	}

	pub := publicKey(label)
	if _, err := raw.ExecContext(ctx, `
INSERT INTO lecturers (id, public_key, display_name) VALUES (?, ?, ?)
`, lecturerID, []byte(pub), name); err != nil {
		return err
	}

	courseID := mustID(courseHex)
	if _, err := raw.ExecContext(ctx, `
INSERT INTO courses (id, code, title, lecturer_id) VALUES (?, ?, ?, ?)
`, courseID, code, title, lecturerID); err != nil {
		return err
	}
	for _, index := range roster {
		if err := db.AddRoster(ctx, courseID, index); err != nil {
			return err
		}
	}

	for _, lecture := range lectures {
		lectureID := mustID(lecture.id)
		var endsAt any
		if lecture.end != nil {
			endsAt = lecture.end.Unix()
		}
		if _, err := raw.ExecContext(ctx, `
INSERT INTO lectures (id, course_id, starts_at, ends_at) VALUES (?, ?, ?, ?)
`, lectureID, courseID, lecture.start.Unix(), endsAt); err != nil {
			return err
		}
		createdAt := lecture.start.Add(5 * time.Minute).Unix()
		for _, mark := range lecture.sensor {
			if _, err := db.SubmitSensor(ctx, store.Attendance{
				IndexNumber: mark.index,
				LectureID:   lectureID,
				Nonce:       demoNonce(mark.index, lecture.id),
				IssuedAt:    lecture.start.Unix(),
				RSSI:        mark.rssi,
				ChirpOK:     mark.chirp,
				CreatedAt:   createdAt,
			}); err != nil {
				return err
			}
		}
		for _, index := range lecture.manual {
			if _, err := db.SubmitManual(ctx, lectureID, index, createdAt); err != nil {
				return err
			}
		}
	}
	return nil
}

func publicKey(label string) ed25519.PublicKey {
	sum := sha256.Sum256([]byte(label))
	priv := ed25519.NewKeyFromSeed(sum[:])
	return priv.Public().(ed25519.PublicKey)
}

func demoNonce(index, lectureHex string) []byte {
	sum := sha256.Sum256([]byte("attendance-demo-nonce-" + index + lectureHex))
	out := make([]byte, 8)
	copy(out, sum[:8])
	return out
}

func mustID(hexID string) []byte {
	b, err := hex.DecodeString(hexID)
	if err != nil {
		panic(err)
	}
	if len(b) != 16 {
		panic(fmt.Sprintf("%s decoded to %d bytes", hexID, len(b)))
	}
	return b
}
