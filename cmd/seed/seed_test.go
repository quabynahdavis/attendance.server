package main

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"attendance/internal/store"
)

func TestSeedTwice(t *testing.T) {
	path := filepath.Join(t.TempDir(), "attendance.db")
	ctx := context.Background()
	db, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	if err := seed(ctx, db, path); err != nil {
		t.Fatal(err)
	}

	courses, err := db.ListCourses(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(courses) != 3 {
		t.Fatalf("courses = %d", len(courses))
	}
	for _, id := range []string{idAma, idKwesi, idLena} {
		if _, err := db.GetLecturer(ctx, mustID(id)); err != nil {
			t.Fatalf("lecturer %s: %v", id, err)
		}
	}
	for _, student := range demoStudents {
		if _, err := db.GetStudent(ctx, student.index); err != nil {
			t.Fatalf("student %s: %v", student.index, err)
		}
	}
	if len(demoStudents) != 8 {
		t.Fatalf("students = %d", len(demoStudents))
	}

	wed := mustID(idLec316Wed)
	wedRows, err := db.ListLectureAttendance(ctx, wed)
	if err != nil {
		t.Fatal(err)
	}
	if len(wedRows) != 0 {
		t.Fatalf("wednesday attendance = %d", len(wedRows))
	}

	mon := mustID(idLec316Mon)
	if _, err := db.GetAttendance(ctx, "10941833", mon); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("Malik Monday CSCD 316: %v", err)
	}
	efua, err := db.GetAttendance(ctx, "10928451", mon)
	if err != nil {
		t.Fatal(err)
	}
	if !efua.ChirpOK {
		t.Fatal("Efua Monday CSCD 316 chirp_ok is unset")
	}
	yaw, err := db.GetAttendance(ctx, "10955290", mon)
	if err != nil {
		t.Fatal(err)
	}
	if yaw.Source != "manual" {
		t.Fatalf("Yaw Monday CSCD 316 source = %s", yaw.Source)
	}

	before := attendanceCount(t, ctx, db)
	if err := seed(ctx, db, path); err != nil {
		t.Fatal(err)
	}
	after := attendanceCount(t, ctx, db)
	if after != before {
		t.Fatalf("attendance rows %d after second seed, was %d", after, before)
	}
}

func attendanceCount(t *testing.T, ctx context.Context, db *store.Store) int {
	t.Helper()
	courses, err := db.ListCourses(ctx)
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, course := range courses {
		lectures, err := db.ListLectures(ctx, course.ID)
		if err != nil {
			t.Fatal(err)
		}
		for _, lecture := range lectures {
			rows, err := db.ListLectureAttendance(ctx, lecture.ID)
			if err != nil {
				t.Fatal(err)
			}
			n += len(rows)
		}
	}
	return n
}
