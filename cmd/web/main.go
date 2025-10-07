package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

type segLocResp struct {
	Video    string   `json:"video"`
	Rend     string   `json:"rend"`
	Idx      int      `json:"idx"`
	Replicas []string `json:"replicas"`
}

func main() {
	metadataBase := os.Getenv("METADATA_BASE")
	if metadataBase == "" {
		metadataBase = "http://localhost:8082"
	}

	writeW := 1
	if ws := os.Getenv("UPLOAD_W"); ws != "" {
		if n, err := strconv.Atoi(ws); err == nil && n > 0 {
			writeW = n
		}
	}
	mux := http.NewServeMux()

	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK"))
	})

	mux.HandleFunc("/upload", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		id := r.URL.Query().Get("id")
		rend := r.URL.Query().Get("rend")
		idx := r.URL.Query().Get("idx")
		if id == "" || rend == "" || idx == "" {
			http.Error(w, "Missing id or rend", http.StatusBadRequest)
			return
		}
		// 1) 先让 metadata 挑副本并登记
		locURL := metadataBase + "/videos/" + id + "/segments?rend=" + rend + "&idx=" + idx
		reqCreate, _ := http.NewRequestWithContext(r.Context(), "POST", locURL, nil)
		respCreate, err := http.DefaultClient.Do(reqCreate)
		if err != nil || respCreate.StatusCode != http.StatusOK {
			if respCreate != nil {
				respCreate.Body.Close()
			}
			http.Error(w, "Failed to create segment", http.StatusServiceUnavailable)
			return
		}
		var loc segLocResp
		if err := json.NewDecoder(respCreate.Body).Decode(&loc); err != nil {
			respCreate.Body.Close()
			http.Error(w, "Failed to decode metadata", http.StatusServiceUnavailable)
			return
		}
		respCreate.Body.Close()
		if len(loc.Replicas) == 0 {
			http.Error(w, "No replicas available", http.StatusServiceUnavailable)
			return
		}

		// 2) 读取请求体（演示用：读入内存；后续会做分块/流式）
		data, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "Failed to read segment", http.StatusServiceUnavailable)
			return
		}

		// 3) 依次 PUT 到各 storage，统计成功数
		objectPath := id + "/" + rend + "/" + idx
		success := 0
		type putResult struct {
			Base   string `json:"base"`
			Status int    `json:"status"`
			Size   int64  `json:"size"`
			SHA256 string `json:"sha256"`
		}
		results := make([]putResult, 0, len(loc.Replicas))

		for _, base := range loc.Replicas {
			u := strings.TrimRight(base, "/") + "/blob/videos/" + objectPath
			sctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
			reqPut, _ := http.NewRequestWithContext(sctx, "PUT", u, bytes.NewReader(data))
			respPut, err := http.DefaultClient.Do(reqPut)
			if err != nil {
				cancel()
				results = append(results, putResult{Base: base, Status: 0})
				continue
			}
			var size int64
			var sum string
			if respPut.StatusCode == http.StatusCreated {
				var m map[string]any
				_ = json.NewDecoder(respPut.Body).Decode(&m)
				if v, ok := m["size"].(float64); ok {
					size = int64(v)
				}
				if s, ok := m["sha256"].(string); ok {
					sum = s
				}
				success++
			}
			respPut.Body.Close()
			cancel()
			results = append(results, putResult{Base: base, Status: respPut.StatusCode, Size: size, SHA256: sum})
		}
		if success >= writeW {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"ok":      true,
				"written": success,
				"W":       writeW,
				"results": results,
				"object":  "videos/" + objectPath,
			})
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadGateway)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok":      false,
			"written": success,
			"W":       writeW,
			"results": results,
		})
	})

	mux.HandleFunc("/v/", func(w http.ResponseWriter, r *http.Request) {
		trim := strings.TrimPrefix(r.URL.Path, "/v/")
		parts := strings.Split(trim, "/")
		if len(parts) != 3 || parts[0] == "" || parts[1] == "" || parts[2] == "" {
			http.Error(w, "bad path, want /v/{id}/{rend}/{idx}", http.StatusBadRequest)
			return
		}
		id, rend, idx := parts[0], parts[1], parts[2]
		locURL := metadataBase + "/videos/" + id + "/segments/loc?rend=" + rend + "&idx=" + idx
		ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		defer cancel()
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, locURL, nil)
		resp, err := http.DefaultClient.Do(req)
		if err != nil || resp.StatusCode != http.StatusOK {
			if resp != nil {
				resp.Body.Close()
			}
			http.Error(w, "metadata error", http.StatusBadGateway)
			return
		}
		var loc segLocResp
		if err := json.NewDecoder(resp.Body).Decode(&loc); err != nil {
			resp.Body.Close()
			http.Error(w, "decode", http.StatusBadGateway)
			return
		}
		resp.Body.Close()
		if len(loc.Replicas) == 0 {
			http.NotFound(w, r)
			return
		}

		objectPath := id + "/" + rend + "/" + idx
		for _, base := range loc.Replicas {
			u := strings.TrimRight(base, "/") + "/blob/videos/" + objectPath
			sctx, scancel := context.WithTimeout(r.Context(), 5*time.Second)
			reqS, _ := http.NewRequestWithContext(sctx, http.MethodGet, u, nil)
			respS, err := http.DefaultClient.Do(reqS)
			if err != nil || respS.StatusCode != http.StatusOK {
				if respS != nil {
					respS.Body.Close()
				}
				scancel()
				continue
			}
			defer respS.Body.Close()
			_, _ = io.Copy(w, respS.Body)
			scancel()
			return
		}
		http.Error(w, "all replicas failed", http.StatusBadGateway)
	})

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("<h1>TritonTube Web</h1><p>Hello!</p>"))
	})

	addr := ":8080"
	log.Printf("Starting server at %s", addr)
	log.Fatal(http.ListenAndServe(addr, mux))
}
