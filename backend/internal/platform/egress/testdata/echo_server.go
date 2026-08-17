package main

import (
	"encoding/json"
	"log"
	"net"
	"net/http"
)

func main() {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host, _, err := net.SplitHostPort(r.RemoteAddr)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"ip": host})
	})
	log.Print("IPv6 echo ready on [::]:8080")
	log.Fatal(http.ListenAndServe("[::]:8080", handler))
}
