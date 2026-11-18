// Package store persists attendance rows in Postgres through database/sql.
package store

import (
	"context"
	"crypto/rand"
	"database/sql"
	"errors"

	"github.com/jackc/pgx/v5/pgconn"
	_ "github.com/jackc/pgx/v5/stdlib"
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

// Store is a Postgres-backed attendance database.
type Store struct {
	db *sql.DB
}

// Open opens DATABASE_URL with the pgx database/sql driver and creates missing tables.
func Open(databaseURL string) (*Store, error) {
	db, err := sql.Open("pgx", databaseURL)
	if err != nil {
		return nil, err
	}
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
  public_key BYTEA NOT NULL,
  display_name TEXT
);
CREATE TABLE IF NOT EXISTS lecturers (
  id BYTEA PRIMARY KEY,
  public_key BYTEA NOT NULL,
  display_name TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS courses (
  id BYTEA PRIMARY KEY,
  code TEXT NOT NULL UNIQUE,
  title TEXT NOT NULL,
  lecturer_id BYTEA NOT NULL REFERENCES lecturers(id)
);
CREATE TABLE IF NOT EXISTS roster (
  course_id BYTEA NOT NULL REFERENCES courses(id),
  index_number TEXT NOT NULL REFERENCES students(index_number),
  PRIMARY KEY (course_id, index_number)
);
CREATE TABLE IF NOT EXISTS lectures (
  id BYTEA PRIMARY KEY,
  course_id BYTEA NOT NULL REFERENCES courses(id),
  starts_at BIGINT NOT NULL,
  ends_at BIGINT
);
CREATE TABLE IF NOT EXISTS attendance (
  index_number TEXT NOT NULL,
  lecture_id BYTEA NOT NULL,
  nonce BYTEA NOT NULL,
  issued_at BIGINT NOT NULL,
  rssi INT NOT NULL,
  chirp_ok BOOLEAN NOT NULL,
  source TEXT NOT NULL CHECK (source IN ('sensor', 'manual')),
  created_at BIGINT NOT NULL,
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
VALUES ($1, $2, $3)
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
	err = s.db.QueryRowContext(ctx, `SELECT public_key FROM students WHERE index_number = $1`, index).Scan(&stored)
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
SELECT index_number, public_key, display_name FROM students WHERE index_number = $1
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
WHERE a.index_number = $1
ORDER BY a.created_at, a.lecture_id
`, index)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []StudentAttendance{}
	for rows.Next() {
		var row StudentAttendance
		if err := rows.Scan(&row.LectureID, &row.CourseID, &row.CourseCode, &row.IssuedAt, &row.ChirpOK, &row.Source, &row.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

// CreateLecturer inserts a lecturer with a fresh 16-byte id.
func (s *Store) CreateLecturer(ctx context.Context, displayName string, publicKey []byte) ([]byte, error) {
	var id []byte
	err := withNewID(func(candidate []byte) error {
		_, err := s.db.ExecContext(ctx, `
INSERT INTO lecturers (id, public_key, display_name) VALUES ($1, $2, $3)
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
SELECT id, public_key, display_name FROM lecturers WHERE id = $1
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
	return s.queryCourses(ctx, `SELECT id, code, title, lecturer_id FROM courses WHERE lecturer_id = $1 ORDER BY code, id`, id)
}

// CreateCourse inserts a course. ErrNotFound if the lecturer is missing. ErrConflict if code exists.
func (s *Store) CreateCourse(ctx context.Context, code, title string, lecturerID []byte) ([]byte, error) {
	if _, err := s.GetLecturer(ctx, lecturerID); err != nil {
		return nil, err
	}
	var id []byte
	err := withNewID(func(candidate []byte) error {
		_, err := s.db.ExecContext(ctx, `
INSERT INTO courses (id, code, title, lecturer_id) VALUES ($1, $2, $3, $4)
`, candidate, code, title, lecturerID)
		if isUnique(err) {
			var exists bool
			qerr := s.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM courses WHERE code = $1)`, code).Scan(&exists)
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
	rows, err := s.queryCourses(ctx, `SELECT id, code, title, lecturer_id FROM courses WHERE id = $1`, id)
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
INSERT INTO roster (course_id, index_number) VALUES ($1, $2)
ON CONFLICT (course_id, index_number) DO NOTHING
`, courseID, index)
	return err
}

// RemoveRoster deletes a roster row. ErrNotFound if the course is missing.
func (s *Store) RemoveRoster(ctx context.Context, courseID []byte, index string) error {
	if _, err := s.GetCourse(ctx, courseID); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx, `DELETE FROM roster WHERE course_id = $1 AND index_number = $2`, courseID, index)
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
WHERE r.course_id = $1
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
	var ok bool
	err := s.db.QueryRowContext(ctx, `
SELECT EXISTS(SELECT 1 FROM roster WHERE course_id = $1 AND index_number = $2)
`, courseID, index).Scan(&ok)
	return ok, err
}

// CreateLecture inserts an open lecture.
func (s *Store) CreateLecture(ctx context.Context, courseID []byte, startsAt int64) ([]byte, error) {
	if _, err := s.GetCourse(ctx, courseID); err != nil {
		return nil, err
	}
	var id []byte
	err := withNewID(func(candidate []byte) error {
		_, err := s.db.ExecContext(ctx, `
INSERT INTO lectures (id, course_id, starts_at, ends_at) VALUES ($1, $2, $3, NULL)
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
SELECT id, course_id, starts_at, ends_at FROM lectures WHERE id = $1
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
SELECT id, course_id, starts_at, ends_at FROM lectures WHERE course_id = $1 ORDER BY starts_at, id
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
UPDATE lectures SET ends_at = $2 WHERE id = $1 AND ends_at IS NULL
RETURNING id, course_id, starts_at, ends_at
`, id, now).Scan(&lec.ID, &lec.CourseID, &lec.StartsAt, &lec.EndsAt)
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
	var out Attendance
	err := s.db.QueryRowContext(ctx, `
INSERT INTO attendance (index_number, lecture_id, nonce, issued_at, rssi, chirp_ok, source, created_at)
VALUES ($1, $2, $3, $4, $5, $6, 'sensor', $7)
ON CONFLICT (index_number, lecture_id) DO UPDATE
SET chirp_ok = TRUE, source = 'sensor'
WHERE attendance.chirp_ok = FALSE AND EXCLUDED.chirp_ok = TRUE
RETURNING index_number, lecture_id, nonce, issued_at, rssi, chirp_ok, source, created_at
`, row.IndexNumber, row.LectureID, row.Nonce, row.IssuedAt, row.RSSI, row.ChirpOK, row.CreatedAt).Scan(
		&out.IndexNumber, &out.LectureID, &out.Nonce, &out.IssuedAt, &out.RSSI, &out.ChirpOK, &out.Source, &out.CreatedAt,
	)
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
	var out Attendance
	err := s.db.QueryRowContext(ctx, `
SELECT index_number, lecture_id, nonce, issued_at, rssi, chirp_ok, source, created_at
FROM attendance WHERE index_number = $1 AND lecture_id = $2
`, index, lectureID).Scan(
		&out.IndexNumber, &out.LectureID, &out.Nonce, &out.IssuedAt, &out.RSSI, &out.ChirpOK, &out.Source, &out.CreatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return Attendance{}, ErrNotFound
	}
	return out, err
}

// SubmitManual inserts a manual row when none exists and otherwise returns the stored row unchanged.
func (s *Store) SubmitManual(ctx context.Context, lectureID []byte, index string, now int64) (Attendance, error) {
	nonce := make([]byte, 8)
	var out Attendance
	err := s.db.QueryRowContext(ctx, `
INSERT INTO attendance (index_number, lecture_id, nonce, issued_at, rssi, chirp_ok, source, created_at)
VALUES ($1, $2, $3, $4, 0, FALSE, 'manual', $4)
ON CONFLICT (index_number, lecture_id) DO NOTHING
RETURNING index_number, lecture_id, nonce, issued_at, rssi, chirp_ok, source, created_at
`, index, lectureID, nonce, now).Scan(
		&out.IndexNumber, &out.LectureID, &out.Nonce, &out.IssuedAt, &out.RSSI, &out.ChirpOK, &out.Source, &out.CreatedAt,
	)
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
WHERE a.lecture_id = $1
ORDER BY a.index_number
`, lectureID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []LectureAttendance{}
	for rows.Next() {
		var row LectureAttendance
		if err := rows.Scan(&row.IndexNumber, &row.DisplayName, &row.RSSI, &row.ChirpOK, &row.Source, &row.CreatedAt); err != nil {
			return nil, err
		}
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
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM lectures WHERE course_id = $1`, courseID).Scan(&held); err != nil {
		return 0, nil, err
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT s.index_number, s.display_name,
  (SELECT COUNT(*) FROM attendance a
    JOIN lectures l ON l.id = a.lecture_id
    WHERE a.index_number = r.index_number AND l.course_id = r.course_id)
FROM roster r
JOIN students s ON s.index_number = r.index_number
WHERE r.course_id = $1
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
	var pg *pgconn.PgError
	return errors.As(err, &pg) && pg.Code == "23505"
}

func isPrimaryKey(err error) bool {
	var pg *pgconn.PgError
	return errors.As(err, &pg) && pg.Code == "23505" && pg.ConstraintName != "" && len(pg.ConstraintName) >= 5 && pg.ConstraintName[len(pg.ConstraintName)-5:] == "_pkey"
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
