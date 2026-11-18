package httpapi

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"attendance/internal/store"
	"attendance/internal/verify"
)

func TestHealthAndAdminToken(t *testing.T) {
	srv := &Server{DB: newMem(), AdminToken: "secret"}
	h := srv.Handler()

	res := do(h, http.MethodGet, "/health", nil, nil)
	if res.Code != 200 || res.Body.String() != "{\"ok\":true}\n" {
		t.Fatalf("health %d %s", res.Code, res.Body.String())
	}

	res = do(h, http.MethodPost, "/lecturers", []byte(`{}`), nil)
	if res.Code != 401 {
		t.Fatalf("missing token %d", res.Code)
	}
	res = do(h, http.MethodPost, "/lecturers", []byte(`not-json`), map[string]string{"Authorization": "Basic secret"})
	if res.Code != 401 {
		t.Fatalf("wrong scheme %d", res.Code)
	}
	res = do(h, http.MethodPost, "/courses", []byte(`not-json`), map[string]string{"Authorization": "Bearer no"})
	if res.Code != 401 {
		t.Fatalf("wrong token %d", res.Code)
	}
	open := &Server{DB: newMem(), AdminToken: ""}
	res = do(open.Handler(), http.MethodPost, "/courses", []byte(`{"code":"c","title":"t","lecturerId":"aaaaaaaaaaaaaaaaaaaaaa=="}`), map[string]string{"Authorization": "Bearer "})
	if res.Code != 401 {
		t.Fatalf("unset token %d", res.Code)
	}
}

func TestStudentClaimAndReads(t *testing.T) {
	mem := newMem()
	h := (&Server{DB: mem}).Handler()
	pub, _, _ := ed25519.GenerateKey(nil)
	body := `{"indexNumber":"cs1","publicKey":"` + b64(pub) + `","displayName":"Ada"}`
	if res := do(h, http.MethodPost, "/students", []byte(body), nil); res.Code != 200 || res.Body.Len() != 0 {
		t.Fatalf("register %d %q", res.Code, res.Body.String())
	}
	again := `{"indexNumber":"cs1","publicKey":"` + b64(pub) + `","displayName":"Other"}`
	if res := do(h, http.MethodPost, "/students", []byte(again), nil); res.Code != 200 {
		t.Fatalf("same key %d", res.Code)
	}
	res := do(h, http.MethodGet, "/students/cs1", nil, nil)
	var got map[string]any
	mustJSON(t, res, 200, &got)
	if got["displayName"] != "Ada" || got["publicKey"] != b64(pub) {
		t.Fatalf("student changed %#v", got)
	}
	other, _, _ := ed25519.GenerateKey(nil)
	clash := `{"indexNumber":"cs1","publicKey":"` + b64(other) + `"}`
	if res := do(h, http.MethodPost, "/students", []byte(clash), nil); res.Code != 409 {
		t.Fatalf("clash %d", res.Code)
	}
	if res := do(h, http.MethodPost, "/students", []byte(`{"indexNumber":"x","publicKey":"YQ=="}`), nil); res.Code != 400 {
		t.Fatalf("short key %d", res.Code)
	}
	if res := do(h, http.MethodGet, "/students/missing", nil, nil); res.Code != 404 {
		t.Fatalf("missing %d", res.Code)
	}
	res = do(h, http.MethodGet, "/students/cs1/attendance", nil, nil)
	var rows []any
	mustJSON(t, res, 200, &rows)
	if len(rows) != 0 {
		t.Fatalf("attendance %#v", rows)
	}
}

func TestLectureSignaturesSensorUpgradeAndManual(t *testing.T) {
	mem := newMem()
	now := int64(1_700_000_000)
	min := -70
	h := (&Server{DB: mem, AdminToken: "secret", RSSIMin: &min, Now: func() int64 { return now }}).Handler()
	auth := map[string]string{"Authorization": "Bearer secret"}

	lecPub, lecPriv, _ := ed25519.GenerateKey(nil)
	stuPub, stuPriv, _ := ed25519.GenerateKey(nil)
	res := do(h, http.MethodPost, "/lecturers", []byte(`{"displayName":"Lee","publicKey":"`+b64(lecPub)+`"}`), auth)
	var idBody map[string]string
	mustJSON(t, res, 200, &idBody)
	lecturerID, _ := base64.StdEncoding.DecodeString(idBody["id"])

	res = do(h, http.MethodPost, "/courses", []byte(`{"code":"CS101","title":"Intro","lecturerId":"`+idBody["id"]+`"}`), auth)
	mustJSON(t, res, 200, &idBody)
	courseID, _ := base64.StdEncoding.DecodeString(idBody["id"])
	coursePath := "/courses/" + idBody["id"]

	if res := do(h, http.MethodPost, "/courses", []byte(`{"code":"CS101","title":"Dup","lecturerId":"`+b64(lecturerID)+`"}`), auth); res.Code != 409 {
		t.Fatalf("dup code %d", res.Code)
	}
	if res := do(h, http.MethodPost, "/students", []byte(`{"indexNumber":"s1","publicKey":"`+b64(stuPub)+`"}`), nil); res.Code != 200 {
		t.Fatal(res.Code)
	}
	if res := do(h, http.MethodPost, coursePath+"/roster", []byte(`{"indexNumber":"s1"}`), auth); res.Code != 200 {
		t.Fatalf("roster %d", res.Code)
	}
	if res := do(h, http.MethodPost, coursePath+"/roster", []byte(`{"indexNumber":"s1"}`), auth); res.Code != 200 {
		t.Fatalf("roster repeat %d", res.Code)
	}

	starts := uint64(now)
	createMsg := verify.LectureCreate(courseID, starts)
	headers := signedHeaders(lecturerID, ed25519.Sign(lecPriv, createMsg))
	res = do(h, http.MethodPost, coursePath+"/lectures", []byte(`{"startsAt":1700000000}`), headers)
	mustJSON(t, res, 200, &idBody)
	lectureID, _ := base64.StdEncoding.DecodeString(idBody["id"])
	lecturePath := "/lectures/" + idBody["id"]

	bad := signedHeaders(lecturerID, ed25519.Sign(lecPriv, []byte("nope")))
	if res := do(h, http.MethodPost, coursePath+"/lectures", []byte(`{"startsAt":1700000000}`), bad); res.Code != 401 {
		t.Fatalf("bad sig %d", res.Code)
	}

	nonce := []byte{1, 2, 3, 4, 5, 6, 7, 8}
	sensor := sensorBody("s1", nonce, uint32(now), lectureID, lecPriv, stuPriv, -80, false)
	if res := do(h, http.MethodPost, lecturePath+"/attendance", sensor, nil); res.Code != 409 {
		t.Fatalf("rssi reject %d %s", res.Code, res.Body.String())
	}
	sensor = sensorBody("s1", nonce, uint32(now), lectureID, lecPriv, stuPriv, -80, true)
	res = do(h, http.MethodPost, lecturePath+"/attendance", sensor, nil)
	var att map[string]any
	mustJSON(t, res, 200, &att)
	if att["chirpOk"] != true || att["source"] != "sensor" || att["confidence"] != "high" || att["rssi"].(float64) != -80 {
		t.Fatalf("first sensor %#v", att)
	}
	created := att["createdAt"]

	sensor = sensorBody("s1", []byte{8, 7, 6, 5, 4, 3, 2, 1}, uint32(now), lectureID, lecPriv, stuPriv, -10, false)
	res = do(h, http.MethodPost, lecturePath+"/attendance", sensor, nil)
	mustJSON(t, res, 200, &att)
	if att["chirpOk"] != true || att["source"] != "sensor" || att["rssi"].(float64) != -80 || att["createdAt"] != created || att["nonce"] != b64(nonce) {
		t.Fatalf("upgrade kept stored fields %#v", att)
	}

	manualHeaders := signedHeaders(lecturerID, ed25519.Sign(lecPriv, verify.Manual(lectureID, "s1")))
	res = do(h, http.MethodPost, lecturePath+"/attendance/manual", []byte(`{"indexNumber":"s1","note":"ignored"}`), manualHeaders)
	mustJSON(t, res, 200, &att)
	if att["source"] != "sensor" || att["chirpOk"] != true || att["rssi"].(float64) != -80 {
		t.Fatalf("manual overwrote sensor %#v", att)
	}

	if res := do(h, http.MethodPost, "/students", []byte(`{"indexNumber":"s2","publicKey":"`+b64(stuPub)+`"}`), nil); res.Code != 200 {
		t.Fatal(res.Code)
	}
	if res := do(h, http.MethodPost, coursePath+"/roster", []byte(`{"indexNumber":"s2"}`), auth); res.Code != 200 {
		t.Fatal(res.Code)
	}
	res = do(h, http.MethodPost, lecturePath+"/attendance/manual", []byte(`{"indexNumber":"s2"}`), signedHeaders(lecturerID, ed25519.Sign(lecPriv, verify.Manual(lectureID, "s2"))))
	mustJSON(t, res, 200, &att)
	if att["source"] != "manual" || att["chirpOk"] != false || att["confidence"] != "low" || att["rssi"].(float64) != 0 || att["nonce"] != b64(make([]byte, 8)) {
		t.Fatalf("manual row %#v", att)
	}

	low := sensorBody("s2", nonce, uint32(now), lectureID, lecPriv, stuPriv, -40, false)
	res = do(h, http.MethodPost, lecturePath+"/attendance", low, nil)
	mustJSON(t, res, 200, &att)
	if att["source"] != "manual" || att["chirpOk"] != false {
		t.Fatalf("false chirp changed manual %#v", att)
	}
	up := sensorBody("s2", nonce, uint32(now), lectureID, lecPriv, stuPriv, -40, true)
	res = do(h, http.MethodPost, lecturePath+"/attendance", up, nil)
	mustJSON(t, res, 200, &att)
	if att["source"] != "sensor" || att["chirpOk"] != true || att["rssi"].(float64) != 0 {
		t.Fatalf("manual chirp upgrade %#v", att)
	}

	endHeaders := signedHeaders(lecturerID, ed25519.Sign(lecPriv, verify.LectureEnd(lectureID)))
	res = do(h, http.MethodPost, lecturePath+"/end", nil, endHeaders)
	var ended map[string]any
	mustJSON(t, res, 200, &ended)
	if ended["endsAt"].(float64) != float64(now) {
		t.Fatalf("end %#v", ended)
	}
	if res := do(h, http.MethodPost, lecturePath+"/end", nil, endHeaders); res.Code != 409 {
		t.Fatalf("second end %d", res.Code)
	}
	if res := do(h, http.MethodPost, lecturePath+"/attendance/manual", []byte(`{"indexNumber":"s2"}`), signedHeaders(lecturerID, ed25519.Sign(lecPriv, verify.Manual(lectureID, "s2")))); res.Code != 409 {
		t.Fatalf("manual after end %d", res.Code)
	}

	res = do(h, http.MethodGet, lecturePath+"/attendance", nil, nil)
	var list map[string]any
	mustJSON(t, res, 200, &list)
	if list["count"].(float64) != 2 {
		t.Fatalf("count %#v", list)
	}
	res = do(h, http.MethodGet, coursePath+"/attendance/summary", nil, nil)
	var summary map[string]any
	mustJSON(t, res, 200, &summary)
	if summary["lecturesHeld"].(float64) != 1 {
		t.Fatalf("summary %#v", summary)
	}
}

func TestSensorWindows(t *testing.T) {
	mem := newMem()
	now := int64(5_000)
	h := (&Server{DB: mem, AdminToken: "t", Now: func() int64 { return now }}).Handler()
	auth := map[string]string{"Authorization": "Bearer t"}
	lecPub, lecPriv, _ := ed25519.GenerateKey(nil)
	stuPub, stuPriv, _ := ed25519.GenerateKey(nil)
	res := do(h, http.MethodPost, "/lecturers", []byte(`{"displayName":"L","publicKey":"`+b64(lecPub)+`"}`), auth)
	var ids map[string]string
	mustJSON(t, res, 200, &ids)
	lecturerID, _ := base64.StdEncoding.DecodeString(ids["id"])
	res = do(h, http.MethodPost, "/courses", []byte(`{"code":"C","title":"T","lecturerId":"`+ids["id"]+`"}`), auth)
	mustJSON(t, res, 200, &ids)
	courseID, _ := base64.StdEncoding.DecodeString(ids["id"])
	do(h, http.MethodPost, "/students", []byte(`{"indexNumber":"s","publicKey":"`+b64(stuPub)+`"}`), nil)
	do(h, http.MethodPost, "/courses/"+b64(courseID)+"/roster", []byte(`{"indexNumber":"s"}`), auth)

	early := now + 301
	res = do(h, http.MethodPost, "/courses/"+b64(courseID)+"/lectures", []byte(`{"startsAt":`+itoa(early)+`}`), signedHeaders(lecturerID, ed25519.Sign(lecPriv, verify.LectureCreate(courseID, uint64(early)))))
	mustJSON(t, res, 200, &ids)
	lectureID, _ := base64.StdEncoding.DecodeString(ids["id"])
	nonce := []byte{1, 1, 1, 1, 1, 1, 1, 1}
	body := sensorBody("s", nonce, uint32(now), lectureID, lecPriv, stuPriv, -20, false)
	if res := do(h, http.MethodPost, "/lectures/"+ids["id"]+"/attendance", body, nil); res.Code != 409 {
		t.Fatalf("too early %d", res.Code)
	}

	edge := now + 300
	res = do(h, http.MethodPost, "/courses/"+b64(courseID)+"/lectures", []byte(`{"startsAt":`+itoa(edge)+`}`), signedHeaders(lecturerID, ed25519.Sign(lecPriv, verify.LectureCreate(courseID, uint64(edge)))))
	mustJSON(t, res, 200, &ids)
	lectureID, _ = base64.StdEncoding.DecodeString(ids["id"])
	body = sensorBody("s", nonce, uint32(now-20), lectureID, lecPriv, stuPriv, -20, false)
	if res := do(h, http.MethodPost, "/lectures/"+ids["id"]+"/attendance", body, nil); res.Code != 200 {
		t.Fatalf("edge window %d %s", res.Code, res.Body.String())
	}
	body = sensorBody("s", nonce, uint32(now-21), lectureID, lecPriv, stuPriv, -20, true)
	if res := do(h, http.MethodPost, "/lectures/"+ids["id"]+"/attendance", body, nil); res.Code != 409 {
		t.Fatalf("stale issuedAt %d", res.Code)
	}
	if res := do(h, http.MethodPost, "/lectures/"+ids["id"]+"/attendance", []byte(`{"indexNumber":"nope"}`), nil); res.Code != 400 {
		t.Fatalf("malformed %d", res.Code)
	}
	if res := do(h, http.MethodGet, "/courses", nil, nil); res.Code != 200 {
		t.Fatal(res.Code)
	}
}

func sensorBody(index string, nonce []byte, issued uint32, lectureID []byte, lecPriv, stuPriv ed25519.PrivateKey, rssi int, chirp bool) []byte {
	lecturerSig := ed25519.Sign(lecPriv, verify.SensorLecturer(lectureID, issued, nonce))
	studentSig := ed25519.Sign(stuPriv, verify.SensorStudent(lectureID, nonce))
	chirpText := "false"
	if chirp {
		chirpText = "true"
	}
	return []byte(`{"indexNumber":"` + index + `","nonce":"` + b64(nonce) + `","issuedAt":` + itoa(int64(issued)) + `,"lecturerSig":"` + b64(lecturerSig) + `","studentSig":"` + b64(studentSig) + `","rssi":` + itoa(int64(rssi)) + `,"chirpOk":` + chirpText + `}`)
}

func signedHeaders(id, sig []byte) map[string]string {
	return map[string]string{
		"X-Lecturer-Id":  b64(id),
		"X-Lecturer-Sig": b64(sig),
	}
}

func do(h http.Handler, method, path string, body []byte, headers map[string]string) *httptest.ResponseRecorder {
	var rdr io.Reader
	if body != nil {
		rdr = bytes.NewReader(body)
	}
	req := httptest.NewRequest(method, path, rdr)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func mustJSON(t *testing.T, res *httptest.ResponseRecorder, code int, dst any) {
	t.Helper()
	if res.Code != code {
		t.Fatalf("status %d body %s", res.Code, res.Body.String())
	}
	if err := json.Unmarshal(res.Body.Bytes(), dst); err != nil {
		t.Fatal(err)
	}
}

func itoa(n int64) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}

type mem struct {
	mu         sync.Mutex
	students   map[string]store.Student
	lecturers  map[string]store.Lecturer
	courses    map[string]store.Course
	roster     map[string]map[string]bool
	lectures   map[string]store.Lecture
	attendance map[string]store.Attendance
	seq        byte
}

func newMem() *mem {
	return &mem{
		students:   map[string]store.Student{},
		lecturers:  map[string]store.Lecturer{},
		courses:    map[string]store.Course{},
		roster:     map[string]map[string]bool{},
		lectures:   map[string]store.Lecture{},
		attendance: map[string]store.Attendance{},
	}
}

func (m *mem) RegisterStudent(_ context.Context, index string, publicKey []byte, displayName *string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if cur, ok := m.students[index]; ok {
		return !bytes.Equal(cur.PublicKey, publicKey), nil
	}
	m.students[index] = store.Student{IndexNumber: index, PublicKey: append([]byte(nil), publicKey...), DisplayName: displayName}
	return false, nil
}

func (m *mem) GetStudent(_ context.Context, index string) (store.Student, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	st, ok := m.students[index]
	if !ok {
		return store.Student{}, store.ErrNotFound
	}
	return st, nil
}

func (m *mem) ListStudentAttendance(_ context.Context, index string) ([]store.StudentAttendance, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.students[index]; !ok {
		return nil, store.ErrNotFound
	}
	out := []store.StudentAttendance{}
	for _, row := range m.attendance {
		if row.IndexNumber != index {
			continue
		}
		lec := m.lectures[key(row.LectureID)]
		course := m.courses[key(lec.CourseID)]
		out = append(out, store.StudentAttendance{
			LectureID: row.LectureID, CourseID: course.ID, CourseCode: course.Code,
			IssuedAt: row.IssuedAt, ChirpOK: row.ChirpOK, Source: row.Source, CreatedAt: row.CreatedAt,
		})
	}
	return out, nil
}

func (m *mem) CreateLecturer(_ context.Context, displayName string, publicKey []byte) ([]byte, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	id := m.nextID()
	m.lecturers[key(id)] = store.Lecturer{ID: id, PublicKey: append([]byte(nil), publicKey...), DisplayName: displayName}
	return id, nil
}

func (m *mem) GetLecturer(_ context.Context, id []byte) (store.Lecturer, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	lec, ok := m.lecturers[key(id)]
	if !ok {
		return store.Lecturer{}, store.ErrNotFound
	}
	return lec, nil
}

func (m *mem) ListLecturerCourses(_ context.Context, id []byte) ([]store.Course, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.lecturers[key(id)]; !ok {
		return nil, store.ErrNotFound
	}
	out := []store.Course{}
	for _, c := range m.courses {
		if bytes.Equal(c.LecturerID, id) {
			out = append(out, c)
		}
	}
	return out, nil
}

func (m *mem) CreateCourse(_ context.Context, code, title string, lecturerID []byte) ([]byte, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.lecturers[key(lecturerID)]; !ok {
		return nil, store.ErrNotFound
	}
	for _, c := range m.courses {
		if c.Code == code {
			return nil, store.ErrConflict
		}
	}
	id := m.nextID()
	m.courses[key(id)] = store.Course{ID: id, Code: code, Title: title, LecturerID: append([]byte(nil), lecturerID...)}
	m.roster[key(id)] = map[string]bool{}
	return id, nil
}

func (m *mem) ListCourses(context.Context) ([]store.Course, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := []store.Course{}
	for _, c := range m.courses {
		out = append(out, c)
	}
	return out, nil
}

func (m *mem) GetCourse(_ context.Context, id []byte) (store.Course, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	c, ok := m.courses[key(id)]
	if !ok {
		return store.Course{}, store.ErrNotFound
	}
	return c, nil
}

func (m *mem) AddRoster(_ context.Context, courseID []byte, index string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.courses[key(courseID)]; !ok {
		return store.ErrNotFound
	}
	if _, ok := m.students[index]; !ok {
		return store.ErrNotFound
	}
	m.roster[key(courseID)][index] = true
	return nil
}

func (m *mem) RemoveRoster(_ context.Context, courseID []byte, index string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	roster, ok := m.roster[key(courseID)]
	if !ok {
		return store.ErrNotFound
	}
	delete(roster, index)
	return nil
}

func (m *mem) ListRoster(_ context.Context, courseID []byte) ([]store.RosterEntry, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	roster, ok := m.roster[key(courseID)]
	if !ok {
		return nil, store.ErrNotFound
	}
	out := []store.RosterEntry{}
	for index := range roster {
		st := m.students[index]
		out = append(out, store.RosterEntry{IndexNumber: st.IndexNumber, DisplayName: st.DisplayName})
	}
	return out, nil
}

func (m *mem) OnRoster(_ context.Context, courseID []byte, index string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.roster[key(courseID)][index], nil
}

func (m *mem) CreateLecture(_ context.Context, courseID []byte, startsAt int64) ([]byte, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.courses[key(courseID)]; !ok {
		return nil, store.ErrNotFound
	}
	id := m.nextID()
	m.lectures[key(id)] = store.Lecture{ID: id, CourseID: append([]byte(nil), courseID...), StartsAt: startsAt}
	return id, nil
}

func (m *mem) GetLecture(_ context.Context, id []byte) (store.Lecture, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	lec, ok := m.lectures[key(id)]
	if !ok {
		return store.Lecture{}, store.ErrNotFound
	}
	return lec, nil
}

func (m *mem) ListLectures(_ context.Context, courseID []byte) ([]store.Lecture, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.courses[key(courseID)]; !ok {
		return nil, store.ErrNotFound
	}
	out := []store.Lecture{}
	for _, lec := range m.lectures {
		if bytes.Equal(lec.CourseID, courseID) {
			out = append(out, lec)
		}
	}
	return out, nil
}

func (m *mem) EndLecture(_ context.Context, id []byte, now int64) (store.Lecture, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	lec, ok := m.lectures[key(id)]
	if !ok {
		return store.Lecture{}, store.ErrNotFound
	}
	if lec.EndsAt != nil {
		return store.Lecture{}, store.ErrConflict
	}
	lec.EndsAt = &now
	m.lectures[key(id)] = lec
	return lec, nil
}

func (m *mem) SubmitSensor(_ context.Context, row store.Attendance) (store.Attendance, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	k := row.IndexNumber + "|" + key(row.LectureID)
	cur, ok := m.attendance[k]
	if !ok {
		row.Source = "sensor"
		row.Nonce = append([]byte(nil), row.Nonce...)
		row.LectureID = append([]byte(nil), row.LectureID...)
		m.attendance[k] = row
		return row, nil
	}
	if !cur.ChirpOK && row.ChirpOK {
		cur.ChirpOK = true
		cur.Source = "sensor"
		m.attendance[k] = cur
	}
	return m.attendance[k], nil
}

func (m *mem) SubmitManual(_ context.Context, lectureID []byte, index string, now int64) (store.Attendance, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	k := index + "|" + key(lectureID)
	if cur, ok := m.attendance[k]; ok {
		return cur, nil
	}
	row := store.Attendance{
		IndexNumber: index,
		LectureID:   append([]byte(nil), lectureID...),
		Nonce:       make([]byte, 8),
		IssuedAt:    now,
		RSSI:        0,
		ChirpOK:     false,
		Source:      "manual",
		CreatedAt:   now,
	}
	m.attendance[k] = row
	return row, nil
}

func (m *mem) ListLectureAttendance(_ context.Context, lectureID []byte) ([]store.LectureAttendance, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.lectures[key(lectureID)]; !ok {
		return nil, store.ErrNotFound
	}
	out := []store.LectureAttendance{}
	for _, row := range m.attendance {
		if !bytes.Equal(row.LectureID, lectureID) {
			continue
		}
		st := m.students[row.IndexNumber]
		out = append(out, store.LectureAttendance{
			IndexNumber: row.IndexNumber, DisplayName: st.DisplayName, RSSI: row.RSSI,
			ChirpOK: row.ChirpOK, Source: row.Source, CreatedAt: row.CreatedAt,
		})
	}
	return out, nil
}

func (m *mem) CourseSummary(_ context.Context, courseID []byte) (int, []store.SummaryStudent, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.courses[key(courseID)]; !ok {
		return 0, nil, store.ErrNotFound
	}
	held := 0
	for _, lec := range m.lectures {
		if bytes.Equal(lec.CourseID, courseID) {
			held++
		}
	}
	out := []store.SummaryStudent{}
	for index := range m.roster[key(courseID)] {
		st := m.students[index]
		attended := 0
		for _, row := range m.attendance {
			if row.IndexNumber != index {
				continue
			}
			lec := m.lectures[key(row.LectureID)]
			if bytes.Equal(lec.CourseID, courseID) {
				attended++
			}
		}
		out = append(out, store.SummaryStudent{IndexNumber: index, DisplayName: st.DisplayName, Attended: attended})
	}
	return held, out, nil
}

func (m *mem) nextID() []byte {
	m.seq++
	id := make([]byte, 16)
	id[15] = m.seq
	return id
}

func key(b []byte) string { return string(b) }
