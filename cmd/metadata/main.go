package main

import (
	"log"
	"net/http"
)

func main() {
	mux := http.NewServeMux()

	// 健康检查
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("ok"))
	})

	// 占位页
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("<h1>TritonTube Metadata</h1>"))
	})

	addr := ":8082"
	log.Printf("metadata listening on %s\n", addr)
	log.Fatal(http.ListenAndServe(addr, mux))
}
