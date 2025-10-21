package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"

	"tritontube/internal/chash"
	"tritontube/internal/metadata"
	"tritontube/internal/metadata/etcdsim"
	"tritontube/internal/metadata/pgxsim"
)

type server struct {
	ring *chash.Ring
	svc  *metadata.Service
}

func main() {
	srv := newServer()

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("ok")) })
	mux.HandleFunc("/videos", srv.handleCreateVideo)
	mux.HandleFunc("/videos/", srv.routeVideo)
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("<h1>TritonTube Metadata gRPC</h1>"))
	})

	addr := ":8082"
	log.Printf("metadata listening on %s (simulated gRPC backend)\n", addr)
	log.Fatal(http.ListenAndServe(addr, mux))
}

func newServer() *server {
	ring := chash.NewRing(128)
	for _, node := range loadNodesFromEnv() {
		ring.AddNode(node)
	}
	store := pgxsim.NewStore()
	pool := pgxsim.NewPool(store)
	etcd, err := etcdsim.New(etcdsim.Config{Endpoints: []string{os.Getenv("ETCD_ENDPOINT")}})
	if err != nil {
		log.Fatalf("failed to init etcd sim: %v", err)
	}
	svc, err := metadata.NewService(metadata.ServiceConfig{WritePool: pool, ReadPool: pool, Etcd: etcd, KeyPrefix: "segments/"})
	if err != nil {
		log.Fatalf("failed to init metadata service: %v", err)
	}
	return &server{ring: ring, svc: svc}
}

func loadNodesFromEnv() []string {
	raw := os.Getenv("STORAGE_NODES")
	if raw == "" {
		raw = "http://localhost:8081,http://localhost:8083"
	}
	var nodes []string
	for _, s := range strings.Split(raw, ",") {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		nodes = append(nodes, s)
	}
	log.Printf("metadata nodes: %v (vnodes=%d)\n", nodes, 128)
	return nodes
}

func (s *server) handleCreateVideo(w http.ResponseWriter, r *http.Request) {
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
	item := &metadata.MetadataItem{Key: "video/" + q.ID, Value: `{"status":"ingesting"}`}
	if _, err := s.svc.PutMetadata(r.Context(), &metadata.PutMetadataRequest{Item: item, ExpectedEtcdRevision: -1}); err != nil {
		http.Error(w, fmt.Sprintf("store video: %v", err), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"id": q.ID, "status": "ingesting"})
}

func (s *server) routeVideo(w http.ResponseWriter, r *http.Request) {
	if strings.HasSuffix(r.URL.Path, "/segments") && r.Method == http.MethodPost {
		s.handlePutSegment(w, r)
		return
	}
	if strings.HasSuffix(r.URL.Path, "/segments/loc") && r.Method == http.MethodGet {
		s.handleGetLocations(w, r)
		return
	}
	http.NotFound(w, r)
}

func (s *server) handlePutSegment(w http.ResponseWriter, r *http.Request) {
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

	keyBytes := []byte(id + "|" + rend + "|" + idxStr)
	replicas := s.ring.Lookup(keyBytes, 2)
	if len(replicas) == 0 {
		http.Error(w, "no storage nodes", http.StatusServiceUnavailable)
		return
	}

	payload, _ := json.Marshal(map[string]any{"video": id, "rend": rend, "idx": idx, "replicas": replicas})
	item := &metadata.MetadataItem{Key: segmentKey(id, rend, idx), Value: string(payload)}

	existing, err := s.svc.GetMetadata(r.Context(), &metadata.GetMetadataRequest{Key: item.Key})
	if err == nil {
		_, err = s.svc.PutMetadata(r.Context(), &metadata.PutMetadataRequest{
			Item:                 item,
			ExpectedVersion:      existing.Item.Version,
			ExpectedEtcdRevision: -1,
		})
	} else {
		_, err = s.svc.PutMetadata(r.Context(), &metadata.PutMetadataRequest{Item: item, ExpectedEtcdRevision: -1})
	}
	if err != nil {
		http.Error(w, fmt.Sprintf("store segment: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"video":    id,
		"rend":     rend,
		"idx":      idx,
		"replicas": replicas,
	})
}

func (s *server) handleGetLocations(w http.ResponseWriter, r *http.Request) {
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

	resp, err := s.svc.GetMetadata(r.Context(), &metadata.GetMetadataRequest{Key: segmentKey(id, rend, idx)})
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	type stored struct {
		Video    string   `json:"video"`
		Rend     string   `json:"rend"`
		Idx      int      `json:"idx"`
		Replicas []string `json:"replicas"`
	}
	var info stored
	_ = json.Unmarshal([]byte(resp.Item.Value), &info)
	if len(info.Replicas) == 0 {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"video":    info.Video,
		"rend":     info.Rend,
		"idx":      info.Idx,
		"replicas": info.Replicas,
	})
}

func segmentKey(id, rend string, idx int) string {
	return fmt.Sprintf("segment/%s/%s/%d", id, rend, idx)
}
