// Package verify builds the attendance signature messages and checks Ed25519 signatures.
package verify

import (
	"crypto/ed25519"
	"encoding/binary"
)

// Signature reports whether sig is a valid Ed25519 signature of message under publicKey.
func Signature(publicKey, message, sig []byte) bool {
	if len(publicKey) != ed25519.PublicKeySize || len(sig) != ed25519.SignatureSize {
		return false
	}
	return ed25519.Verify(publicKey, message, sig)
}

// LectureCreate is the 39-byte message for POST /courses/{id}/lectures.
// Layout: ASCII "lecture.create\n" || courseID (16) || startsAt (uint64 big-endian).
func LectureCreate(courseID []byte, startsAt uint64) []byte {
	msg := make([]byte, 15+16+8)
	copy(msg, "lecture.create\n")
	copy(msg[15:31], courseID)
	binary.BigEndian.PutUint64(msg[31:], startsAt)
	return msg
}

// LectureEnd is the 28-byte message for POST /lectures/{id}/end.
// Layout: ASCII "lecture.end\n" || lectureID (16).
func LectureEnd(lectureID []byte) []byte {
	msg := make([]byte, 12+16)
	copy(msg, "lecture.end\n")
	copy(msg[12:], lectureID)
	return msg
}

// Manual is the message for POST /lectures/{id}/attendance/manual.
// Layout: ASCII "attendance.manual\n" || lectureID (16) || indexNumber as UTF-8.
func Manual(lectureID []byte, indexNumber string) []byte {
	idx := []byte(indexNumber)
	msg := make([]byte, 18+16+len(idx))
	copy(msg, "attendance.manual\n")
	copy(msg[18:34], lectureID)
	copy(msg[34:], idx)
	return msg
}

// SensorLecturer is the 28-byte lecturer message for sensor attendance.
// Layout: lectureID (16) || issuedAt (uint32 big-endian) || nonce (8).
func SensorLecturer(lectureID []byte, issuedAt uint32, nonce []byte) []byte {
	msg := make([]byte, 16+4+8)
	copy(msg, lectureID)
	binary.BigEndian.PutUint32(msg[16:], issuedAt)
	copy(msg[20:], nonce)
	return msg
}

// SensorStudent is the 24-byte student message for sensor attendance.
// Layout: lectureID (16) || nonce (8).
func SensorStudent(lectureID []byte, nonce []byte) []byte {
	msg := make([]byte, 16+8)
	copy(msg, lectureID)
	copy(msg[16:], nonce)
	return msg
}
