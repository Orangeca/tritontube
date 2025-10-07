package main

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"os"
	"strings"

	st "tritontube/internal/storage"
)

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8081"
	}

	root := os.Getenv("DATA_DIR")
	if root == "" {
		root = "./data-" + port
	}
	store := st.NewFS(root)

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		_, _ = w.Write([]byte("OK"))
	})

	mux.HandleFunc("/blob/", func(w http.ResponseWriter, r *http.Request) {
		trim := strings.TrimPrefix(r.URL.Path, "/blob/")
		parts := strings.SplitN(trim, "/", 2)
		if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
			http.Error(w, "Invalid path", http.StatusBadRequest)
			return
		}
		bucket, object := parts[0], parts[1]

		switch r.Method {
		case http.MethodPut:
			n, sum, err := store.Put(bucket, object, r.Body)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"size":   n,
				"sha256": sum,
				"bucket": bucket,
				"object": object,
			})
		case http.MethodGet:
			rc, err := store.Get(bucket, object)
			if err != nil {
				http.NotFound(w, r)
				return
			}
			defer rc.Close()
			if _, err := io.Copy(w, rc); err != nil {
				log.Printf("stream error: %v", err)
			}
		default:
			http.Error(w, "Unsupported method", http.StatusMethodNotAllowed)
		}
	})

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("<h1>TritonTube Storage</h1>"))
	})

	log.Printf("storage listening on :%s\n", port)
	log.Fatal(http.ListenAndServe(":"+port, mux))
}
