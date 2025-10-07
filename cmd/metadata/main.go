package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"

	"tritontube/internal/chash" // 确保和你的 go.mod 模块名一致
)

type segKey struct {
	videoID   string
	rendition string
	idx       int
}

var (
	mu         sync.RWMutex
	placements = map[segKey][]string{} // 仍用内存表记录（后续落 PG）
	ring       = chash.NewRing(128)    // 每个节点 128 个虚拟节点
	nodes      []string                // 只是为了日志打印
)

func loadNodesFromEnv() {
	raw := os.Getenv("STORAGE_NODES")
	if raw == "" {
		raw = "http://localhost:8081,http://localhost:8083"
	}
	for _, s := range strings.Split(raw, ",") {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		nodes = append(nodes, s)
		ring.AddNode(s) // 直接用 baseURL 当 nodeID
	}
	log.Printf("metadata nodes: %v (vnodes=%d)\n", nodes, 128)
}

// 创建视频（最小：仅回个 id）
func handleCreateVideo(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
		return
	}
	type req struct {
		ID string `json:"id"`
	}
	var q req
	_ = json.NewDecoder(r.Body).Decode(&q)
	if q.ID == "" {
		http.Error(w, "missing id", http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"id": q.ID, "status": "ingesting"})
}

// 记录分片放置：POST /videos/{id}/segments?rend=720p&idx=0
// 返回本分片应写入的 storage baseURLs（副本列表）
func handlePutSegment(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
		return
	}
	p := strings.TrimPrefix(r.URL.Path, "/videos/")
	parts := strings.SplitN(p, "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] != "segments" {
		http.Error(w, "bad path", http.StatusBadRequest)
		return
	}
	id := parts[0]
	rend := r.URL.Query().Get("rend")
	idxStr := r.URL.Query().Get("idx")
	if id == "" || rend == "" || idxStr == "" {
		http.Error(w, "need id,rend,idx", http.StatusBadRequest)
		return
	}
	var idx int
	if _, err := fmt.Sscanf(idxStr, "%d", &idx); err != nil {
		http.Error(w, "bad idx", http.StatusBadRequest)
		return
	}

	// 用一致性哈希选择 2 个副本（相同 key 稳定映射）
	keyBytes := []byte(id + "|" + rend + "|" + idxStr)
	replicas := ring.Lookup(keyBytes, 2)
	if len(replicas) == 0 {
		http.Error(w, "no storage nodes", http.StatusServiceUnavailable)
		return
	}

	// 记录放置（内存版）
	key := segKey{videoID: id, rendition: rend, idx: idx}
	mu.Lock()
	placements[key] = replicas
	mu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"video":    id,
		"rend":     rend,
		"idx":      idx,
		"replicas": replicas,
	})
}

// 查询分片位置：GET /videos/{id}/segments/loc?rend=720p&idx=0
func handleGetLocations(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
		return
	}
	p := strings.TrimPrefix(r.URL.Path, "/videos/")
	parts := strings.SplitN(p, "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] != "segments/loc" {
		http.Error(w, "bad path", http.StatusBadRequest)
		return
	}
	id := parts[0]
	rend := r.URL.Query().Get("rend")
	idxStr := r.URL.Query().Get("idx")
	if id == "" || rend == "" || idxStr == "" {
		http.Error(w, "need id,rend,idx", http.StatusBadRequest)
		return
	}
	var idx int
	if _, err := fmt.Sscanf(idxStr, "%d", &idx); err != nil {
		http.Error(w, "bad idx", http.StatusBadRequest)
		return
	}

	key := segKey{videoID: id, rendition: rend, idx: idx}
	mu.RLock()
	reps := placements[key]
	mu.RUnlock()
	if len(reps) == 0 {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"video":    id,
		"rend":     rend,
		"idx":      idx,
		"replicas": reps,
	})
}

func main() {
	loadNodesFromEnv()

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("ok")) })
	mux.HandleFunc("/videos", handleCreateVideo)
	mux.HandleFunc("/videos/", func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/segments") && r.Method == http.MethodPost {
			handlePutSegment(w, r)
			return
		}
		if strings.HasSuffix(r.URL.Path, "/segments/loc") && r.Method == http.MethodGet {
			handleGetLocations(w, r)
			return
		}
		http.NotFound(w, r)
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("<h1>TritonTube Metadata</h1>"))
	})

	addr := ":8082"
	log.Printf("metadata listening on %s\n", addr)
	log.Fatal(http.ListenAndServe(addr, mux))
}
