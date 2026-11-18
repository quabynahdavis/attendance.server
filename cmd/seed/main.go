package main

import (
	"context"
	"encoding/base64"
	"fmt"
	"log"
	"os"
	"strings"

	"attendance/internal/store"
)

func main() {
	dbPath := os.Getenv("ATTENDANCE_DB")
	if dbPath == "" {
		dbPath = "attendance.db"
	}
	db, err := store.Open(dbPath)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	ctx := context.Background()
	if err := seed(ctx, db, dbPath); err != nil {
		log.Fatal(err)
	}
	if err := printSummary(ctx, db); err != nil {
		log.Fatal(err)
	}
}

func printSummary(ctx context.Context, db *store.Store) error {
	courses, err := db.ListCourses(ctx)
	if err != nil {
		return err
	}
	fmt.Println("code\tlecture_id\topen\tpresent")
	for _, course := range courses {
		lectures, err := db.ListLectures(ctx, course.ID)
		if err != nil {
			return err
		}
		for _, lecture := range lectures {
			rows, err := db.ListLectureAttendance(ctx, lecture.ID)
			if err != nil {
				return err
			}
			open := "closed"
			if lecture.EndsAt == nil {
				open = "open"
			}
			present := make([]string, len(rows))
			for i, row := range rows {
				present[i] = row.IndexNumber
			}
			fmt.Printf("%s\t%s\t%s\t%s\n", course.Code, base64.StdEncoding.EncodeToString(lecture.ID), open, strings.Join(present, ","))
		}
	}

	fmt.Println("lecturer_id\tpublic_key")
	for _, id := range []string{idAma, idKwesi, idLena} {
		lecturer, err := db.GetLecturer(ctx, mustID(id))
		if err != nil {
			return err
		}
		fmt.Printf("%s\t%s\n", base64.StdEncoding.EncodeToString(lecturer.ID), base64.StdEncoding.EncodeToString(lecturer.PublicKey))
	}

	fmt.Println("index_number\tpublic_key")
	for _, student := range demoStudents {
		row, err := db.GetStudent(ctx, student.index)
		if err != nil {
			return err
		}
		fmt.Printf("%s\t%s\n", row.IndexNumber, base64.StdEncoding.EncodeToString(row.PublicKey))
	}
	return nil
}
