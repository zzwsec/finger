package main

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"strconv"
)

type cdnResponse struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func main() {
	address := os.Getenv("DEV_CDN_ADDRESS")
	if address == "" {
		address = ":20011"
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/openserver/", handleOpenServer)

	log.Printf("test CDN server listening on %s", address)
	log.Fatal(http.ListenAndServe(address, mux))
}

func handleOpenServer(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")

	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		writeJSON(w, cdnResponse{Code: 1, Message: "method not allowed"})
		return
	}

	zoneID := r.URL.Query().Get("zone_id")
	zone, err := strconv.Atoi(zoneID)
	if err != nil || zone <= 0 {
		w.WriteHeader(http.StatusBadRequest)
		writeJSON(w, cdnResponse{Code: 1, Message: "zone_id must be a positive integer"})
		return
	}

	log.Printf("received CDN flush request: zone_id=%d remote=%s", zone, r.RemoteAddr)
	writeJSON(w, cdnResponse{Code: 0, Message: "success"})
}

func writeJSON(w http.ResponseWriter, body cdnResponse) {
	if err := json.NewEncoder(w).Encode(body); err != nil {
		log.Printf("write response: %v", err)
	}
}
