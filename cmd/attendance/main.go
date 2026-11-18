package main

import (
	"log"
	"net/http"
	"os"
	"strconv"

	"attendance/internal/httpapi"
	"attendance/internal/store"
)

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	dbPath := os.Getenv("ATTENDANCE_DB")
	if dbPath == "" {
		dbPath = "attendance.db"
	}
	db, err := store.Open(dbPath)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	var rssiMin *int
	if raw, ok := os.LookupEnv("ATTENDANCE_RSSI_MIN"); ok && raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil {
			log.Fatal("ATTENDANCE_RSSI_MIN must be an integer")
		}
		rssiMin = &n
	}

	srv := &httpapi.Server{
		DB:         db,
		AdminToken: os.Getenv("ATTENDANCE_ADMIN_TOKEN"),
		RSSIMin:    rssiMin,
	}
	log.Fatal(http.ListenAndServe(":"+port, srv.Handler()))
}
