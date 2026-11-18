// Package store persists attendance rows in SQLite through database/sql.
package store

import (
	"context"
	"crypto/rand"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"modernc.org/sqlite"
)

// ErrNotFound is returned when a required row does not exist.
var ErrNotFound = errors.New("not found")

// ErrConflict is returned when a unique value is already stored.
var ErrConflict = errors.New("conflict")

// Student is one students row.
type Student struct {
	IndexNumber string
	PublicKey   []byte
	DisplayName *string
}

// Lecturer is one lecturers row.
type Lecturer struct {
	ID          []byte
	PublicKey   []byte
	DisplayName string
}

// Course is one courses row.
type Course struct {
	ID         []byte
	Code       string
	Title      string
	LecturerID []byte
}

// Lecture is one lectures row. EndsAt is nil while the lecture is open.
type Lecture struct {
	ID       []byte
	CourseID []byte
	StartsAt int64
	EndsAt   *int64
}

// Attendance is one attendance row.
type Attendance struct {
	IndexNumber string
	LectureID   []byte
	Nonce       []byte
	IssuedAt    int64
	RSSI        int
	ChirpOK     bool
	Source      string
	CreatedAt   int64
}

// RosterEntry is a student on a course roster.
type RosterEntry struct {
	IndexNumber string
	DisplayName *string
}

// StudentAttendance is one attendance row joined to its course.
type StudentAttendance struct {
	LectureID  []byte
	CourseID   []byte
	CourseCode string
	IssuedAt   int64
	ChirpOK    bool
	Source     string
	CreatedAt  int64
}

// LectureAttendance is one row of a lecture attendance list.
type LectureAttendance struct {
	IndexNumber string
	DisplayName *string
	RSSI        int
	ChirpOK     bool
	Source      string
	CreatedAt   int64
}

// SummaryStudent is one roster student's attendance count for a course.
type SummaryStudent struct {
	IndexNumber string
	DisplayName *string
	Attended    int
}

// Store is a SQLite-backed attendance database.
type Store struct {
	db *sql.DB
}

// Open opens a SQLite file and creates missing tables.
// An empty path uses attendance.db. Foreign keys and a busy timeout are enabled on each connection.
func Open(path string) (*Store, error) {
	if path == "" {
		path = "attendance.db"
	}
	dsn := path + "?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	s := &Store{db: db}
	if err := s.migrate(context.Background()); err != nil {
		_ = db.Close()
		return nil, err
	}
	return s, nil
}

// Close closes the database handle.
func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) migrate(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `
CREATE TABLE IF NOT EXISTS students (
  index_number TEXT PRIMARY KEY,
  public_key BLOB NOT NULL,
  display_name TEXT
);
CREATE TABLE IF NOT EXISTS lecturers (
  id BLOB PRIMARY KEY,
  public_key BLOB NOT NULL,
  display_name TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS courses (
  id BLOB PRIMARY KEY,
  code TEXT NOT NULL UNIQUE,
  title TEXT NOT NULL,
  lecturer_id BLOB NOT NULL REFERENCES lecturers(id)
);
CREATE TABLE IF NOT EXISTS roster (
  course_id BLOB NOT NULL REFERENCES courses(id),
  index_number TEXT NOT NULL REFERENCES students(index_number),
  PRIMARY KEY (course_id, index_number)
);
CREATE TABLE IF NOT EXISTS lectures (
  id BLOB PRIMARY KEY,
  course_id BLOB NOT NULL REFERENCES courses(id),
  starts_at INTEGER NOT NULL,
  ends_at INTEGER
);
CREATE TABLE IF NOT EXISTS attendance (
  index_number TEXT NOT NULL,
  lecture_id BLOB NOT NULL,
  nonce BLOB NOT NULL,
  issued_at INTEGER NOT NULL,
  rssi INTEGER NOT NULL,
  chirp_ok INTEGER NOT NULL,
  source TEXT NOT NULL CHECK (source IN ('sensor', 'manual')),
  created_at INTEGER NOT NULL,
  PRIMARY KEY (index_number, lecture_id)
);
`)
	return err
}

// RegisterStudent stores the first key for an index number.
// conflict is true when a different key is already stored.
// A matching key leaves the row unchanged.
func (s *Store) RegisterStudent(ctx context.Context, index string, publicKey []byte, displayName *string) (bool, error) {
	res, err := s.db.ExecContext(ctx, `
INSERT INTO students (index_number, public_key, display_name)
VALUES (?, ?, ?)
ON CONFLICT (index_number) DO NOTHING
`, index, publicKey, displayName)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	if n == 1 {
		return false, nil
	}
	var stored []byte
	err = s.db.QueryRowContext(ctx, `SELECT public_key FROM students WHERE index_number = ?`, index).Scan(&stored)
	if err != nil {
		return false, err
	}
	if bytesEqual(stored, publicKey) {
		return false, nil
	}
	return true, nil
}

// GetStudent returns one student.
func (s *Store) GetStudent(ctx context.Context, index string) (Student, error) {
	var st Student
	err := s.db.QueryRowContext(ctx, `
SELECT index_number, public_key, display_name FROM students WHERE index_number = ?
`, index).Scan(&st.IndexNumber, &st.PublicKey, &st.DisplayName)
	if errors.Is(err, sql.ErrNoRows) {
		return Student{}, ErrNotFound
	}
	return st, err
}

// ListStudentAttendance returns attendance for a student. ErrNotFound if the student is missing.
func (s *Store) ListStudentAttendance(ctx context.Context, index string) ([]StudentAttendance, error) {
	if _, err := s.GetStudent(ctx, index); err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT a.lecture_id, l.course_id, c.code, a.issued_at, a.chirp_ok, a.source, a.created_at
FROM attendance a
JOIN lectures l ON l.id = a.lecture_id
JOIN courses c ON c.id = l.course_id
WHERE a.index_number = ?
ORDER BY a.created_at, a.lecture_id
`, index)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []StudentAttendance{}
	for rows.Next() {
		var row StudentAttendance
		var chirp intBool
		if err := rows.Scan(&row.LectureID, &row.CourseID, &row.CourseCode, &row.IssuedAt, &chirp, &row.Source, &row.CreatedAt); err != nil {
			return nil, err
		}
		row.ChirpOK = bool(chirp)
		out = append(out, row)
	}
	return out, rows.Err()
}

// CreateLecturer inserts a lecturer with a fresh 16-byte id.
func (s *Store) CreateLecturer(ctx context.Context, displayName string, publicKey []byte) ([]byte, error) {
	var id []byte
	err := withNewID(func(candidate []byte) error {
		_, err := s.db.ExecContext(ctx, `
INSERT INTO lecturers (id, public_key, display_name) VALUES (?, ?, ?)
`, candidate, publicKey, displayName)
		if err != nil {
			return err
		}
		id = append([]byte(nil), candidate...)
		return nil
	})
	return id, err
}

// GetLecturer returns one lecturer.
func (s *Store) GetLecturer(ctx context.Context, id []byte) (Lecturer, error) {
	var lec Lecturer
	err := s.db.QueryRowContext(ctx, `
SELECT id, public_key, display_name FROM lecturers WHERE id = ?
`, id).Scan(&lec.ID, &lec.PublicKey, &lec.DisplayName)
	if errors.Is(err, sql.ErrNoRows) {
		return Lecturer{}, ErrNotFound
	}
	return lec, err
}

// ListLecturerCourses returns courses taught by a lecturer. ErrNotFound if the lecturer is missing.
func (s *Store) ListLecturerCourses(ctx context.Context, id []byte) ([]Course, error) {
	if _, err := s.GetLecturer(ctx, id); err != nil {
		return nil, err
	}
	return s.queryCourses(ctx, `SELECT id, code, title, lecturer_id FROM courses WHERE lecturer_id = ? ORDER BY code, id`, id)
}

// CreateCourse inserts a course. ErrNotFound if the lecturer is missing. ErrConflict if code exists.
func (s *Store) CreateCourse(ctx context.Context, code, title string, lecturerID []byte) ([]byte, error) {
	if _, err := s.GetLecturer(ctx, lecturerID); err != nil {
		return nil, err
	}
	var id []byte
	err := withNewID(func(candidate []byte) error {
		_, err := s.db.ExecContext(ctx, `
INSERT INTO courses (id, code, title, lecturer_id) VALUES (?, ?, ?, ?)
`, candidate, code, title, lecturerID)
		if isUnique(err) {
			var exists intBool
			qerr := s.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM courses WHERE code = ?)`, code).Scan(&exists)
			if qerr != nil {
				return qerr
			}
			if exists {
				return ErrConflict
			}
			return errPrimary
		}
		if err != nil {
			return err
		}
		id = append([]byte(nil), candidate...)
		return nil
	})
	return id, err
}

// ListCourses returns every course.
func (s *Store) ListCourses(ctx context.Context) ([]Course, error) {
	return s.queryCourses(ctx, `SELECT id, code, title, lecturer_id FROM courses ORDER BY code, id`)
}

// GetCourse returns one course.
func (s *Store) GetCourse(ctx context.Context, id []byte) (Course, error) {
	rows, err := s.queryCourses(ctx, `SELECT id, code, title, lecturer_id FROM courses WHERE id = ?`, id)
	if err != nil {
		return Course{}, err
	}
	if len(rows) == 0 {
		return Course{}, ErrNotFound
	}
	return rows[0], nil
}

// AddRoster inserts a roster row. A duplicate roster row is not an error.
func (s *Store) AddRoster(ctx context.Context, courseID []byte, index string) error {
	if _, err := s.GetCourse(ctx, courseID); err != nil {
		return err
	}
	if _, err := s.GetStudent(ctx, index); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx, `
INSERT INTO roster (course_id, index_number) VALUES (?, ?)
ON CONFLICT (course_id, index_number) DO NOTHING
`, courseID, index)
	return err
}

// RemoveRoster deletes a roster row. ErrNotFound if the course is missing.
func (s *Store) RemoveRoster(ctx context.Context, courseID []byte, index string) error {
	if _, err := s.GetCourse(ctx, courseID); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx, `DELETE FROM roster WHERE course_id = ? AND index_number = ?`, courseID, index)
	return err
}

// ListRoster returns the roster of a course. ErrNotFound if the course is missing.
func (s *Store) ListRoster(ctx context.Context, courseID []byte) ([]RosterEntry, error) {
	if _, err := s.GetCourse(ctx, courseID); err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT s.index_number, s.display_name
FROM roster r
JOIN students s ON s.index_number = r.index_number
WHERE r.course_id = ?
ORDER BY s.index_number
`, courseID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []RosterEntry{}
	for rows.Next() {
		var row RosterEntry
		if err := rows.Scan(&row.IndexNumber, &row.DisplayName); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

// OnRoster reports whether the student is on the course roster.
func (s *Store) OnRoster(ctx context.Context, courseID []byte, index string) (bool, error) {
	var ok intBool
	err := s.db.QueryRowContext(ctx, `
SELECT EXISTS(SELECT 1 FROM roster WHERE course_id = ? AND index_number = ?)
`, courseID, index).Scan(&ok)
	return bool(ok), err
}

// CreateLecture inserts an open lecture.
func (s *Store) CreateLecture(ctx context.Context, courseID []byte, startsAt int64) ([]byte, error) {
	if _, err := s.GetCourse(ctx, courseID); err != nil {
		return nil, err
	}
	var id []byte
	err := withNewID(func(candidate []byte) error {
		_, err := s.db.ExecContext(ctx, `
INSERT INTO lectures (id, course_id, starts_at, ends_at) VALUES (?, ?, ?, NULL)
`, candidate, courseID, startsAt)
		if err != nil {
			return err
		}
		id = append([]byte(nil), candidate...)
		return nil
	})
	return id, err
}

// GetLecture returns one lecture.
func (s *Store) GetLecture(ctx context.Context, id []byte) (Lecture, error) {
	var lec Lecture
	err := s.db.QueryRowContext(ctx, `
SELECT id, course_id, starts_at, ends_at FROM lectures WHERE id = ?
`, id).Scan(&lec.ID, &lec.CourseID, &lec.StartsAt, &lec.EndsAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Lecture{}, ErrNotFound
	}
	return lec, err
}

// ListLectures returns lectures of a course. ErrNotFound if the course is missing.
func (s *Store) ListLectures(ctx context.Context, courseID []byte) ([]Lecture, error) {
	if _, err := s.GetCourse(ctx, courseID); err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT id, course_id, starts_at, ends_at FROM lectures WHERE course_id = ? ORDER BY starts_at, id
`, courseID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Lecture{}
	for rows.Next() {
		var lec Lecture
		if err := rows.Scan(&lec.ID, &lec.CourseID, &lec.StartsAt, &lec.EndsAt); err != nil {
			return nil, err
		}
		out = append(out, lec)
	}
	return out, rows.Err()
}

// EndLecture sets ends_at when the lecture is still open. ErrConflict if it has ended.
func (s *Store) EndLecture(ctx context.Context, id []byte, now int64) (Lecture, error) {
	var lec Lecture
	err := s.db.QueryRowContext(ctx, `
UPDATE lectures SET ends_at = ? WHERE id = ? AND ends_at IS NULL
RETURNING id, course_id, starts_at, ends_at
`, now, id).Scan(&lec.ID, &lec.CourseID, &lec.StartsAt, &lec.EndsAt)
	if err == nil {
		return lec, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return Lecture{}, err
	}
	if _, gerr := s.GetLecture(ctx, id); gerr != nil {
		return Lecture{}, gerr
	}
	return Lecture{}, ErrConflict
}

// SubmitSensor inserts a sensor attendance row or applies the chirp upgrade.
// The returned row is the stored row after the upgrade rule.
func (s *Store) SubmitSensor(ctx context.Context, row Attendance) (Attendance, error) {
	out, err := s.scanAttendance(s.db.QueryRowContext(ctx, `
INSERT INTO attendance (index_number, lecture_id, nonce, issued_at, rssi, chirp_ok, source, created_at)
VALUES (?, ?, ?, ?, ?, ?, 'sensor', ?)
ON CONFLICT (index_number, lecture_id) DO UPDATE
SET chirp_ok = 1, source = 'sensor'
WHERE attendance.chirp_ok = 0 AND excluded.chirp_ok = 1
RETURNING index_number, lecture_id, nonce, issued_at, rssi, chirp_ok, source, created_at
`, row.IndexNumber, row.LectureID, row.Nonce, row.IssuedAt, row.RSSI, boolInt(row.ChirpOK), row.CreatedAt))
	if err == nil {
		return out, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return Attendance{}, err
	}
	return s.GetAttendance(ctx, row.IndexNumber, row.LectureID)
}

// GetAttendance returns one attendance row.
func (s *Store) GetAttendance(ctx context.Context, index string, lectureID []byte) (Attendance, error) {
	out, err := s.scanAttendance(s.db.QueryRowContext(ctx, `
SELECT index_number, lecture_id, nonce, issued_at, rssi, chirp_ok, source, created_at
FROM attendance WHERE index_number = ? AND lecture_id = ?
`, index, lectureID))
	if errors.Is(err, sql.ErrNoRows) {
		return Attendance{}, ErrNotFound
	}
	return out, err
}

// SubmitManual inserts a manual row when none exists and otherwise returns the stored row unchanged.
func (s *Store) SubmitManual(ctx context.Context, lectureID []byte, index string, now int64) (Attendance, error) {
	nonce := make([]byte, 8)
	out, err := s.scanAttendance(s.db.QueryRowContext(ctx, `
INSERT INTO attendance (index_number, lecture_id, nonce, issued_at, rssi, chirp_ok, source, created_at)
VALUES (?, ?, ?, ?, 0, 0, 'manual', ?)
ON CONFLICT (index_number, lecture_id) DO NOTHING
RETURNING index_number, lecture_id, nonce, issued_at, rssi, chirp_ok, source, created_at
`, index, lectureID, nonce, now, now))
	if err == nil {
		return out, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return Attendance{}, err
	}
	return s.GetAttendance(ctx, index, lectureID)
}

// ListLectureAttendance returns attendance for a lecture. ErrNotFound if the lecture is missing.
func (s *Store) ListLectureAttendance(ctx context.Context, lectureID []byte) ([]LectureAttendance, error) {
	if _, err := s.GetLecture(ctx, lectureID); err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT a.index_number, s.display_name, a.rssi, a.chirp_ok, a.source, a.created_at
FROM attendance a
JOIN students s ON s.index_number = a.index_number
WHERE a.lecture_id = ?
ORDER BY a.index_number
`, lectureID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []LectureAttendance{}
	for rows.Next() {
		var row LectureAttendance
		var chirp intBool
		if err := rows.Scan(&row.IndexNumber, &row.DisplayName, &row.RSSI, &chirp, &row.Source, &row.CreatedAt); err != nil {
			return nil, err
		}
		row.ChirpOK = bool(chirp)
		out = append(out, row)
	}
	return out, rows.Err()
}

// CourseSummary returns lecturesHeld and one count per roster student.
func (s *Store) CourseSummary(ctx context.Context, courseID []byte) (int, []SummaryStudent, error) {
	if _, err := s.GetCourse(ctx, courseID); err != nil {
		return 0, nil, err
	}
	var held int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM lectures WHERE course_id = ?`, courseID).Scan(&held); err != nil {
		return 0, nil, err
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT s.index_number, s.display_name,
  (SELECT COUNT(*) FROM attendance a
    JOIN lectures l ON l.id = a.lecture_id
    WHERE a.index_number = r.index_number AND l.course_id = r.course_id)
FROM roster r
JOIN students s ON s.index_number = r.index_number
WHERE r.course_id = ?
ORDER BY s.index_number
`, courseID)
	if err != nil {
		return 0, nil, err
	}
	defer rows.Close()
	out := []SummaryStudent{}
	for rows.Next() {
		var row SummaryStudent
		if err := rows.Scan(&row.IndexNumber, &row.DisplayName, &row.Attended); err != nil {
			return 0, nil, err
		}
		out = append(out, row)
	}
	return held, out, rows.Err()
}

func (s *Store) queryCourses(ctx context.Context, query string, args ...any) ([]Course, error) {
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Course{}
	for rows.Next() {
		var c Course
		if err := rows.Scan(&c.ID, &c.Code, &c.Title, &c.LecturerID); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (s *Store) scanAttendance(row *sql.Row) (Attendance, error) {
	var out Attendance
	var chirp intBool
	err := row.Scan(&out.IndexNumber, &out.LectureID, &out.Nonce, &out.IssuedAt, &out.RSSI, &chirp, &out.Source, &out.CreatedAt)
	out.ChirpOK = bool(chirp)
	return out, err
}

var errPrimary = errors.New("primary key collision")

func withNewID(insert func([]byte) error) error {
	for i := 0; i < 5; i++ {
		id := make([]byte, 16)
		if _, err := rand.Read(id); err != nil {
			return err
		}
		err := insert(id)
		if errors.Is(err, errPrimary) || isPrimaryKey(err) {
			continue
		}
		return err
	}
	return errors.New("could not allocate id")
}

func isUnique(err error) bool {
	se, ok := sqliteErr(err)
	if !ok {
		return false
	}
	code := se.Code()
	if code != 19 && code != 1555 && code != 2067 {
		return strings.Contains(se.Error(), "UNIQUE constraint failed")
	}
	return true
}

func isPrimaryKey(err error) bool {
	se, ok := sqliteErr(err)
	if !ok {
		return false
	}
	if se.Code() == 1555 {
		return true
	}
	msg := se.Error()
	return strings.Contains(msg, "PRIMARY KEY") ||
		strings.Contains(msg, "lecturers.id") ||
		strings.Contains(msg, "courses.id") ||
		strings.Contains(msg, "lectures.id")
}

func sqliteErr(err error) (*sqlite.Error, bool) {
	var se *sqlite.Error
	if errors.As(err, &se) {
		return se, true
	}
	return nil, false
}

func boolInt(v bool) int {
	if v {
		return 1
	}
	return 0
}

type intBool bool

func (b *intBool) Scan(src any) error {
	switch v := src.(type) {
	case bool:
		*b = intBool(v)
	case int64:
		*b = intBool(v != 0)
	case int:
		*b = intBool(v != 0)
	case []byte:
		*b = intBool(len(v) > 0 && v[0] != '0' && string(v) != "false")
	case string:
		*b = intBool(v != "" && v != "0" && !strings.EqualFold(v, "false"))
	default:
		return fmt.Errorf("scan bool from %T", src)
	}
	return nil
}

func bytesEqual(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	diff := 0
	for i := range a {
		diff |= int(a[i] ^ b[i])
	}
	return diff == 0
}
