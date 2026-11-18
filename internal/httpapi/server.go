// Package httpapi serves the attendance HTTP contract.
package httpapi

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"math"
	"net/http"
	"strconv"
	"time"

	"attendance/internal/store"
	"attendance/internal/verify"
)

// Server is the attendance HTTP API.
type Server struct {
	DB         Backend
	AdminToken string
	RSSIMin    *int
	Now        func() int64
}

// Backend is the persistence surface used by the handlers.
type Backend interface {
	RegisterStudent(ctx context.Context, index string, publicKey []byte, displayName *string) (conflict bool, err error)
	GetStudent(ctx context.Context, index string) (store.Student, error)
	ListStudentAttendance(ctx context.Context, index string) ([]store.StudentAttendance, error)
	CreateLecturer(ctx context.Context, displayName string, publicKey []byte) ([]byte, error)
	GetLecturer(ctx context.Context, id []byte) (store.Lecturer, error)
	ListLecturerCourses(ctx context.Context, id []byte) ([]store.Course, error)
	CreateCourse(ctx context.Context, code, title string, lecturerID []byte) ([]byte, error)
	ListCourses(ctx context.Context) ([]store.Course, error)
	GetCourse(ctx context.Context, id []byte) (store.Course, error)
	AddRoster(ctx context.Context, courseID []byte, index string) error
	RemoveRoster(ctx context.Context, courseID []byte, index string) error
	ListRoster(ctx context.Context, courseID []byte) ([]store.RosterEntry, error)
	OnRoster(ctx context.Context, courseID []byte, index string) (bool, error)
	CreateLecture(ctx context.Context, courseID []byte, startsAt int64) ([]byte, error)
	GetLecture(ctx context.Context, id []byte) (store.Lecture, error)
	ListLectures(ctx context.Context, courseID []byte) ([]store.Lecture, error)
	EndLecture(ctx context.Context, id []byte, now int64) (store.Lecture, error)
	SubmitSensor(ctx context.Context, row store.Attendance) (store.Attendance, error)
	SubmitManual(ctx context.Context, lectureID []byte, index string, now int64) (store.Attendance, error)
	ListLectureAttendance(ctx context.Context, lectureID []byte) ([]store.LectureAttendance, error)
	CourseSummary(ctx context.Context, courseID []byte) (int, []store.SummaryStudent, error)
}

// Handler returns the route mux.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", s.health)
	mux.HandleFunc("POST /students", s.postStudent)
	mux.HandleFunc("GET /students/{indexNumber}/attendance", s.getStudentAttendance)
	mux.HandleFunc("GET /students/{indexNumber}", s.getStudent)
	mux.HandleFunc("POST /lecturers", s.postLecturer)
	mux.HandleFunc("GET /lecturers/{id}/courses", s.getLecturerCourses)
	mux.HandleFunc("GET /lecturers/{id}", s.getLecturer)
	mux.HandleFunc("POST /courses", s.postCourse)
	mux.HandleFunc("GET /courses", s.getCourses)
	mux.HandleFunc("POST /courses/{id}/roster", s.postRoster)
	mux.HandleFunc("DELETE /courses/{id}/roster/{indexNumber}", s.deleteRoster)
	mux.HandleFunc("GET /courses/{id}/roster", s.getRoster)
	mux.HandleFunc("POST /courses/{id}/lectures", s.postLecture)
	mux.HandleFunc("GET /courses/{id}/lectures", s.getLectures)
	mux.HandleFunc("GET /courses/{id}/attendance/summary", s.getSummary)
	mux.HandleFunc("GET /courses/{id}", s.getCourse)
	mux.HandleFunc("POST /lectures/{id}/end", s.postLectureEnd)
	mux.HandleFunc("POST /lectures/{id}/attendance/manual", s.postManual)
	mux.HandleFunc("POST /lectures/{id}/attendance", s.postSensor)
	mux.HandleFunc("GET /lectures/{id}/attendance", s.getLectureAttendance)
	mux.HandleFunc("GET /lectures/{id}", s.getLecture)
	return mux
}

func (s *Server) now() int64 {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now().Unix()
}

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) postStudent(w http.ResponseWriter, r *http.Request) {
	var body struct {
		IndexNumber *string `json:"indexNumber"`
		PublicKey   *string `json:"publicKey"`
		DisplayName *string `json:"displayName"`
	}
	if !readJSON(r, &body) || body.IndexNumber == nil || body.PublicKey == nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	key, ok := decodeLen(*body.PublicKey, 32)
	if !ok {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	conflict, err := s.DB.RegisterStudent(r.Context(), *body.IndexNumber, key, body.DisplayName)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	if conflict {
		w.WriteHeader(http.StatusConflict)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func (s *Server) getStudent(w http.ResponseWriter, r *http.Request) {
	st, err := s.DB.GetStudent(r.Context(), r.PathValue("indexNumber"))
	if errors.Is(err, store.ErrNotFound) {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, studentJSON(st))
}

func (s *Server) getStudentAttendance(w http.ResponseWriter, r *http.Request) {
	rows, err := s.DB.ListStudentAttendance(r.Context(), r.PathValue("indexNumber"))
	if errors.Is(err, store.ErrNotFound) {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	out := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		out = append(out, map[string]any{
			"lectureId":  b64(row.LectureID),
			"courseId":   b64(row.CourseID),
			"courseCode": row.CourseCode,
			"issuedAt":   row.IssuedAt,
			"chirpOk":    row.ChirpOK,
			"source":     row.Source,
			"confidence": confidence(row.ChirpOK),
			"createdAt":  row.CreatedAt,
		})
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) postLecturer(w http.ResponseWriter, r *http.Request) {
	if !s.admin(w, r) {
		return
	}
	var body struct {
		DisplayName *string `json:"displayName"`
		PublicKey   *string `json:"publicKey"`
	}
	if !readJSON(r, &body) || body.DisplayName == nil || body.PublicKey == nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	key, ok := decodeLen(*body.PublicKey, 32)
	if !ok {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	id, err := s.DB.CreateLecturer(r.Context(), *body.DisplayName, key)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"id": b64(id)})
}

func (s *Server) getLecturer(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r, "id")
	if !ok {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	lec, err := s.DB.GetLecturer(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{
		"id":          b64(lec.ID),
		"publicKey":   b64(lec.PublicKey),
		"displayName": lec.DisplayName,
	})
}

func (s *Server) getLecturerCourses(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r, "id")
	if !ok {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	rows, err := s.DB.ListLecturerCourses(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, coursesBrief(rows))
}

func (s *Server) postCourse(w http.ResponseWriter, r *http.Request) {
	if !s.admin(w, r) {
		return
	}
	var body struct {
		Code       *string `json:"code"`
		Title      *string `json:"title"`
		LecturerID *string `json:"lecturerId"`
	}
	if !readJSON(r, &body) || body.Code == nil || body.Title == nil || body.LecturerID == nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	lecturerID, ok := decodeLen(*body.LecturerID, 16)
	if !ok {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	id, err := s.DB.CreateCourse(r.Context(), *body.Code, *body.Title, lecturerID)
	if errors.Is(err, store.ErrNotFound) {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	if errors.Is(err, store.ErrConflict) {
		w.WriteHeader(http.StatusConflict)
		return
	}
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"id": b64(id)})
}

func (s *Server) getCourses(w http.ResponseWriter, r *http.Request) {
	rows, err := s.DB.ListCourses(r.Context())
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	out := make([]map[string]string, 0, len(rows))
	for _, c := range rows {
		out = append(out, courseJSON(c))
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) getCourse(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r, "id")
	if !ok {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	c, err := s.DB.GetCourse(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, courseJSON(c))
}

func (s *Server) postRoster(w http.ResponseWriter, r *http.Request) {
	if !s.admin(w, r) {
		return
	}
	courseID, ok := pathID(r, "id")
	if !ok {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	var body struct {
		IndexNumber *string `json:"indexNumber"`
	}
	if !readJSON(r, &body) || body.IndexNumber == nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	err := s.DB.AddRoster(r.Context(), courseID, *body.IndexNumber)
	if errors.Is(err, store.ErrNotFound) {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func (s *Server) deleteRoster(w http.ResponseWriter, r *http.Request) {
	if !s.admin(w, r) {
		return
	}
	courseID, ok := pathID(r, "id")
	if !ok {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	err := s.DB.RemoveRoster(r.Context(), courseID, r.PathValue("indexNumber"))
	if errors.Is(err, store.ErrNotFound) {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) getRoster(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r, "id")
	if !ok {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	rows, err := s.DB.ListRoster(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	out := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		out = append(out, map[string]any{
			"indexNumber": row.IndexNumber,
			"displayName": row.DisplayName,
		})
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) postLecture(w http.ResponseWriter, r *http.Request) {
	courseID, ok := pathID(r, "id")
	if !ok {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	var body struct {
		StartsAt json.RawMessage `json:"startsAt"`
	}
	if !readJSON(r, &body) {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	startsAt, ok := parseUint(body.StartsAt, 64)
	if !ok {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	course, err := s.DB.GetCourse(r.Context(), courseID)
	if errors.Is(err, store.ErrNotFound) {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	if !s.lecturerOK(w, r, course.LecturerID, verify.LectureCreate(courseID, startsAt)) {
		return
	}
	if startsAt > uint64(math.MaxInt64) {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	id, err := s.DB.CreateLecture(r.Context(), courseID, int64(startsAt))
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"id": b64(id)})
}

func (s *Server) getLectures(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r, "id")
	if !ok {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	rows, err := s.DB.ListLectures(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	out := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		out = append(out, lectureBrief(row))
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) getLecture(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r, "id")
	if !ok {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	lec, err := s.DB.GetLecture(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, lectureJSON(lec))
}

func (s *Server) postLectureEnd(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r, "id")
	if !ok {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	lec, err := s.DB.GetLecture(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	course, err := s.DB.GetCourse(r.Context(), lec.CourseID)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	if !s.lecturerOK(w, r, course.LecturerID, verify.LectureEnd(id)) {
		return
	}
	updated, err := s.DB.EndLecture(r.Context(), id, s.now())
	if errors.Is(err, store.ErrConflict) {
		w.WriteHeader(http.StatusConflict)
		return
	}
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, lectureJSON(updated))
}

func (s *Server) postSensor(w http.ResponseWriter, r *http.Request) {
	lectureID, idOK := pathID(r, "id")
	var body struct {
		IndexNumber *string         `json:"indexNumber"`
		Nonce       *string         `json:"nonce"`
		IssuedAt    json.RawMessage `json:"issuedAt"`
		LecturerSig *string         `json:"lecturerSig"`
		StudentSig  *string         `json:"studentSig"`
		RSSI        json.RawMessage `json:"rssi"`
		ChirpOK     *bool           `json:"chirpOk"`
	}
	if !idOK || !readJSON(r, &body) || body.IndexNumber == nil || body.Nonce == nil || body.LecturerSig == nil || body.StudentSig == nil || body.ChirpOK == nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	nonce, nonceOK := decodeLen(*body.Nonce, 8)
	lecturerSig, lecOK := decodeLen(*body.LecturerSig, 64)
	studentSig, stuOK := decodeLen(*body.StudentSig, 64)
	issuedAt, issuedOK := parseUint(body.IssuedAt, 32)
	rssi, rssiOK := parseInt(body.RSSI, 32)
	if !nonceOK || !lecOK || !stuOK || !issuedOK || !rssiOK {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	student, err := s.DB.GetStudent(r.Context(), *body.IndexNumber)
	if errors.Is(err, store.ErrNotFound) {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	lec, err := s.DB.GetLecture(r.Context(), lectureID)
	if errors.Is(err, store.ErrNotFound) {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	now := s.now()
	if lec.EndsAt != nil || (now < lec.StartsAt && lec.StartsAt-now > 300) {
		w.WriteHeader(http.StatusConflict)
		return
	}
	on, err := s.DB.OnRoster(r.Context(), lec.CourseID, student.IndexNumber)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	if !on {
		w.WriteHeader(http.StatusForbidden)
		return
	}
	if !verify.Signature(student.PublicKey, verify.SensorStudent(lectureID, nonce), studentSig) {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	course, err := s.DB.GetCourse(r.Context(), lec.CourseID)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	lecturer, err := s.DB.GetLecturer(r.Context(), course.LecturerID)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	if !verify.Signature(lecturer.PublicKey, verify.SensorLecturer(lectureID, uint32(issuedAt), nonce), lecturerSig) {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	if abs64(now-int64(issuedAt)) > 20 {
		w.WriteHeader(http.StatusConflict)
		return
	}
	if s.RSSIMin != nil && !*body.ChirpOK && rssi < *s.RSSIMin {
		w.WriteHeader(http.StatusConflict)
		return
	}
	stored, err := s.DB.SubmitSensor(r.Context(), store.Attendance{
		IndexNumber: student.IndexNumber,
		LectureID:   lectureID,
		Nonce:       nonce,
		IssuedAt:    int64(issuedAt),
		RSSI:        rssi,
		ChirpOK:     *body.ChirpOK,
		Source:      "sensor",
		CreatedAt:   now,
	})
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, attendanceJSON(stored))
}

func (s *Server) postManual(w http.ResponseWriter, r *http.Request) {
	lectureID, ok := pathID(r, "id")
	var body struct {
		IndexNumber *string `json:"indexNumber"`
		Note        *string `json:"note"`
	}
	if !ok || !readJSON(r, &body) || body.IndexNumber == nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	lec, err := s.DB.GetLecture(r.Context(), lectureID)
	if errors.Is(err, store.ErrNotFound) {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	course, err := s.DB.GetCourse(r.Context(), lec.CourseID)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	if !s.lecturerOK(w, r, course.LecturerID, verify.Manual(lectureID, *body.IndexNumber)) {
		return
	}
	if lec.EndsAt != nil {
		w.WriteHeader(http.StatusConflict)
		return
	}
	on, err := s.DB.OnRoster(r.Context(), course.ID, *body.IndexNumber)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	if !on {
		w.WriteHeader(http.StatusForbidden)
		return
	}
	stored, err := s.DB.SubmitManual(r.Context(), lectureID, *body.IndexNumber, s.now())
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, attendanceJSON(stored))
}

func (s *Server) getLectureAttendance(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r, "id")
	if !ok {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	rows, err := s.DB.ListLectureAttendance(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	out := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		out = append(out, map[string]any{
			"indexNumber": row.IndexNumber,
			"displayName": row.DisplayName,
			"rssi":        row.RSSI,
			"chirpOk":     row.ChirpOK,
			"source":      row.Source,
			"confidence":  confidence(row.ChirpOK),
			"createdAt":   row.CreatedAt,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"count": len(out), "rows": out})
}

func (s *Server) getSummary(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r, "id")
	if !ok {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	held, students, err := s.DB.CourseSummary(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	out := make([]map[string]any, 0, len(students))
	for _, st := range students {
		percent := 0.0
		if held > 0 {
			percent = float64(st.Attended) / float64(held)
		}
		out = append(out, map[string]any{
			"indexNumber": st.IndexNumber,
			"displayName": st.DisplayName,
			"attended":    st.Attended,
			"percent":     percent,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"lecturesHeld": held,
		"students":     out,
	})
}

func (s *Server) admin(w http.ResponseWriter, r *http.Request) bool {
	const prefix = "Bearer "
	header := r.Header.Get("Authorization")
	token := ""
	if len(header) >= len(prefix) && header[:len(prefix)] == prefix {
		token = header[len(prefix):]
	} else {
		w.WriteHeader(http.StatusUnauthorized)
		return false
	}
	if s.AdminToken == "" || subtle.ConstantTimeCompare(hash(token), hash(s.AdminToken)) != 1 {
		w.WriteHeader(http.StatusUnauthorized)
		return false
	}
	return true
}

func (s *Server) lecturerOK(w http.ResponseWriter, r *http.Request, courseLecturer, message []byte) bool {
	id, sig, ok := lecturerHeaders(r)
	if !ok || !bytesEq(id, courseLecturer) {
		w.WriteHeader(http.StatusUnauthorized)
		return false
	}
	lec, err := s.DB.GetLecturer(r.Context(), id)
	if err != nil || !verify.Signature(lec.PublicKey, message, sig) {
		w.WriteHeader(http.StatusUnauthorized)
		return false
	}
	return true
}

func lecturerHeaders(r *http.Request) ([]byte, []byte, bool) {
	id, err1 := base64.StdEncoding.DecodeString(r.Header.Get("X-Lecturer-Id"))
	sig, err2 := base64.StdEncoding.DecodeString(r.Header.Get("X-Lecturer-Sig"))
	if err1 != nil || err2 != nil || len(id) != 16 || len(sig) != 64 {
		return nil, nil, false
	}
	return id, sig, true
}

func hash(s string) []byte {
	sum := sha256.Sum256([]byte(s))
	return sum[:]
}

func readJSON(r *http.Request, dst any) bool {
	dec := json.NewDecoder(r.Body)
	if err := dec.Decode(dst); err != nil {
		return false
	}
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		return false
	}
	return true
}

func pathID(r *http.Request, name string) ([]byte, bool) {
	return decodeLen(r.PathValue(name), 16)
}

func decodeLen(s string, n int) ([]byte, bool) {
	b, err := base64.StdEncoding.DecodeString(s)
	if err != nil || len(b) != n {
		return nil, false
	}
	return b, true
}

func parseUint(raw json.RawMessage, bitSize int) (uint64, bool) {
	if !wholeNumber(raw) {
		return 0, false
	}
	n, err := strconv.ParseUint(string(raw), 10, bitSize)
	if err != nil {
		return 0, false
	}
	return n, true
}

func parseInt(raw json.RawMessage, bitSize int) (int, bool) {
	if !wholeNumber(raw) {
		return 0, false
	}
	n, err := strconv.ParseInt(string(raw), 10, bitSize)
	if err != nil {
		return 0, false
	}
	return int(n), true
}

func wholeNumber(raw json.RawMessage) bool {
	if len(raw) == 0 || string(raw) == "null" {
		return false
	}
	for _, c := range raw {
		if c == '.' || c == 'e' || c == 'E' || c == '"' {
			return false
		}
	}
	return true
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func b64(b []byte) string {
	return base64.StdEncoding.EncodeToString(b)
}

func confidence(chirp bool) string {
	if chirp {
		return "high"
	}
	return "low"
}

func studentJSON(st store.Student) map[string]any {
	return map[string]any{
		"indexNumber": st.IndexNumber,
		"publicKey":   b64(st.PublicKey),
		"displayName": st.DisplayName,
	}
}

func courseJSON(c store.Course) map[string]string {
	return map[string]string{
		"id":         b64(c.ID),
		"code":       c.Code,
		"title":      c.Title,
		"lecturerId": b64(c.LecturerID),
	}
}

func coursesBrief(rows []store.Course) []map[string]string {
	out := make([]map[string]string, 0, len(rows))
	for _, c := range rows {
		out = append(out, map[string]string{
			"id":    b64(c.ID),
			"code":  c.Code,
			"title": c.Title,
		})
	}
	return out
}

func lectureJSON(lec store.Lecture) map[string]any {
	return map[string]any{
		"id":       b64(lec.ID),
		"courseId": b64(lec.CourseID),
		"startsAt": lec.StartsAt,
		"endsAt":   lec.EndsAt,
	}
}

func lectureBrief(lec store.Lecture) map[string]any {
	return map[string]any{
		"id":       b64(lec.ID),
		"startsAt": lec.StartsAt,
		"endsAt":   lec.EndsAt,
	}
}

func attendanceJSON(row store.Attendance) map[string]any {
	return map[string]any{
		"indexNumber": row.IndexNumber,
		"lectureId":   b64(row.LectureID),
		"nonce":       b64(row.Nonce),
		"issuedAt":    row.IssuedAt,
		"rssi":        row.RSSI,
		"chirpOk":     row.ChirpOK,
		"source":      row.Source,
		"createdAt":   row.CreatedAt,
		"confidence":  confidence(row.ChirpOK),
	}
}

func abs64(n int64) int64 {
	if n < 0 {
		return -n
	}
	return n
}

func bytesEq(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	return subtle.ConstantTimeCompare(a, b) == 1
}
