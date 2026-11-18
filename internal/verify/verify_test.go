package verify

import (
	"crypto/ed25519"
	"encoding/binary"
	"testing"
)

func TestMessageLayouts(t *testing.T) {
	course := bytes16(1)
	lecture := bytes16(2)
	nonce := []byte{1, 2, 3, 4, 5, 6, 7, 8}

	create := LectureCreate(course, 0x0102030405060708)
	if len(create) != 39 {
		t.Fatalf("lecture create length %d", len(create))
	}
	if string(create[:15]) != "lecture.create\n" {
		t.Fatalf("lecture create prefix %q", create[:15])
	}
	if got := binary.BigEndian.Uint64(create[31:]); got != 0x0102030405060708 {
		t.Fatalf("startsAt %x", got)
	}

	end := LectureEnd(lecture)
	if len(end) != 28 || string(end[:12]) != "lecture.end\n" {
		t.Fatalf("lecture end %q len %d", end, len(end))
	}

	manual := Manual(lecture, "CS/42")
	if len(manual) != 18+16+len("CS/42") || string(manual[:18]) != "attendance.manual\n" {
		t.Fatalf("manual layout")
	}
	if string(manual[34:]) != "CS/42" {
		t.Fatalf("index %q", manual[34:])
	}

	lecturer := SensorLecturer(lecture, 0x0A0B0C0D, nonce)
	if len(lecturer) != 28 {
		t.Fatalf("sensor lecturer length %d", len(lecturer))
	}
	if got := binary.BigEndian.Uint32(lecturer[16:]); got != 0x0A0B0C0D {
		t.Fatalf("issuedAt %x", got)
	}
	student := SensorStudent(lecture, nonce)
	if len(student) != 24 || string(student[16:]) != string(nonce) {
		t.Fatalf("student message")
	}
}

func TestSignatureRoundTrip(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	otherPub, _, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	lecture := bytes16(9)
	msg := LectureEnd(lecture)
	sig := ed25519.Sign(priv, msg)
	if !Signature(pub, msg, sig) {
		t.Fatal("expected valid signature")
	}
	if Signature(otherPub, msg, sig) {
		t.Fatal("other key accepted")
	}
	tampered := append([]byte(nil), msg...)
	tampered[len(tampered)-1] ^= 0x01
	if Signature(pub, tampered, sig) {
		t.Fatal("tampered message accepted")
	}
	if Signature(pub, msg, sig[:63]) {
		t.Fatal("short signature accepted")
	}
	if Signature(pub[:31], msg, sig) {
		t.Fatal("short key accepted")
	}

	nonce := []byte{9, 8, 7, 6, 5, 4, 3, 2}
	sensor := SensorLecturer(lecture, 20, nonce)
	sensorSig := ed25519.Sign(priv, sensor)
	if !Signature(pub, sensor, sensorSig) {
		t.Fatal("sensor lecturer signature")
	}
	student := SensorStudent(lecture, nonce)
	studentSig := ed25519.Sign(priv, student)
	if !Signature(pub, student, studentSig) {
		t.Fatal("sensor student signature")
	}
	manual := Manual(lecture, "index-1")
	if !Signature(pub, manual, ed25519.Sign(priv, manual)) {
		t.Fatal("manual signature")
	}
	create := LectureCreate(bytes16(3), 1_700_000_000)
	if !Signature(pub, create, ed25519.Sign(priv, create)) {
		t.Fatal("create signature")
	}
}

func bytes16(b byte) []byte {
	out := make([]byte, 16)
	for i := range out {
		out[i] = b
	}
	return out
}
