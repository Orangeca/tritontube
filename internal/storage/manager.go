package storage

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"

	"tritontube/internal/chash"
	"tritontube/internal/metadata/etcdsim"
)

// NodeDescriptor captures the state that each storage node reports via heartbeat.
type NodeDescriptor struct {
	ID             string    `json:"id"`
	Address        string    `json:"address"`
	CapacityBytes  int64     `json:"capacity_bytes"`
	AvailableBytes int64     `json:"available_bytes"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// VirtualNodeAssignment represents the ownership of a token on the consistent hash ring.
type VirtualNodeAssignment struct {
	ID     string `json:"id"`
	Token  uint64 `json:"token"`
	NodeID string `json:"node_id"`
}

type ringState struct {
	Version int64                     `json:"version"`
	Nodes   map[string]NodeDescriptor `json:"nodes"`
	Tokens  []VirtualNodeAssignment   `json:"tokens"`
}

// RingManager persists and watches the consistent hash ring in etcd.
type RingManager struct {
	mu           sync.RWMutex
	ring         *chash.Ring
	state        ringState
	etcd         *etcdsim.Client
	prefix       string
	vnodes       int
	lastRevision int64
}

// RingManagerConfig controls the behaviour of the ring manager.
type RingManagerConfig struct {
	Etcd         *etcdsim.Client
	Prefix       string
	VirtualNodes int
}

// NewRingManager constructs a manager backed by etcd.
func NewRingManager(cfg RingManagerConfig) (*RingManager, error) {
	if cfg.Etcd == nil {
		return nil, errors.New("storage: etcd client is required")
	}
	if cfg.Prefix == "" {
		cfg.Prefix = "/storage/cluster"
	}
	if cfg.VirtualNodes <= 0 {
		cfg.VirtualNodes = 128
	}
	m := &RingManager{
		etcd:   cfg.Etcd,
		prefix: cfg.Prefix,
		vnodes: cfg.VirtualNodes,
	}
	m.state = ringState{Nodes: map[string]NodeDescriptor{}}
	if err := m.bootstrap(context.Background()); err != nil {
		return nil, err
	}
	return m, nil
}

func (m *RingManager) ringKey() string {
	return fmt.Sprintf("%s/ring", m.prefix)
}

func (m *RingManager) bootstrap(ctx context.Context) error {
	resp, err := m.etcd.Get(ctx, m.ringKey())
	if err != nil {
		return err
	}
	if len(resp.KVs) == 0 {
		m.ring = chash.NewRing(m.vnodes)
		return nil
	}
	var state ringState
	if err := json.Unmarshal([]byte(resp.KVs[0].Value), &state); err != nil {
		return fmt.Errorf("storage: failed to decode ring state: %w", err)
	}
	if state.Nodes == nil {
		state.Nodes = map[string]NodeDescriptor{}
	}
	m.state = state
	m.lastRevision = resp.KVs[0].ModRevision
	m.rebuildLocked()
	return nil
}

// UpsertNode registers or updates information about a physical node. It rebuilds the
// ring and persists the new state to etcd.
func (m *RingManager) UpsertNode(ctx context.Context, node NodeDescriptor) (int64, error) {
	if node.ID == "" {
		return 0, errors.New("storage: node ID is required")
	}
	node.UpdatedAt = time.Now().UTC()

	m.mu.Lock()
	defer m.mu.Unlock()

	if m.state.Nodes == nil {
		m.state.Nodes = map[string]NodeDescriptor{}
	}
	m.state.Nodes[node.ID] = node
	m.rebuildLocked()
	if err := m.persistLocked(ctx); err != nil {
		return 0, err
	}
	return m.state.Version, nil
}

// RemoveNode removes a node from the ring.
func (m *RingManager) RemoveNode(ctx context.Context, nodeID string) (int64, error) {
	if nodeID == "" {
		return 0, errors.New("storage: node ID is required")
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	if m.state.Nodes == nil {
		return m.state.Version, nil
	}
	if _, ok := m.state.Nodes[nodeID]; !ok {
		return m.state.Version, nil
	}
	delete(m.state.Nodes, nodeID)
	m.rebuildLocked()
	if err := m.persistLocked(ctx); err != nil {
		return 0, err
	}
	return m.state.Version, nil
}

func (m *RingManager) persistLocked(ctx context.Context) error {
	m.state.Version++
	encoded, err := json.Marshal(m.state)
	if err != nil {
		return err
	}
	resp, err := m.etcd.Put(ctx, m.ringKey(), string(encoded))
	if err != nil {
		return err
	}
	m.lastRevision = resp.Revision
	return nil
}

func (m *RingManager) rebuildLocked() {
	ring := chash.NewRing(m.vnodes)
	ids := make([]string, 0, len(m.state.Nodes))
	for id := range m.state.Nodes {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		ring.AddNode(id)
	}
	tokens := ring.Tokens()
	assignments := make([]VirtualNodeAssignment, len(tokens))
	counter := map[string]int{}
	for i, tok := range tokens {
		idx := counter[tok.Node]
		counter[tok.Node] = idx + 1
		assignments[i] = VirtualNodeAssignment{
			ID:     fmt.Sprintf("%s#%d", tok.Node, idx),
			Token:  tok.Hash,
			NodeID: tok.Node,
		}
	}
	sort.Slice(assignments, func(i, j int) bool {
		if assignments[i].Token == assignments[j].Token {
			return assignments[i].ID < assignments[j].ID
		}
		return assignments[i].Token < assignments[j].Token
	})
	m.state.Tokens = assignments
	m.ring = ring
}

// Lookup resolves the replica set for the provided key.
func (m *RingManager) Lookup(key []byte, replicas int) []string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.ring == nil {
		return nil
	}
	return m.ring.Lookup(key, replicas)
}

// Assignments returns a copy of the current token assignment along with the logical version.
func (m *RingManager) Assignments() ([]VirtualNodeAssignment, int64) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]VirtualNodeAssignment, len(m.state.Tokens))
	copy(out, m.state.Tokens)
	return out, m.state.Version
}

// RingEvent describes a change observed via etcd watch.
type RingEvent struct {
	Version     int64
	Assignments []VirtualNodeAssignment
}

// Watch emits ring events whenever the underlying etcd key changes. The latest state is
// also applied locally so callers may rely on Lookup after receiving an event.
func (m *RingManager) Watch(ctx context.Context) (<-chan RingEvent, error) {
	events := make(chan RingEvent, 8)
	watchCh := m.etcd.Watch(ctx, m.prefix)

	go func() {
		defer close(events)
		for {
			select {
			case <-ctx.Done():
				return
			case resp, ok := <-watchCh:
				if !ok {
					return
				}
				for _, evt := range resp.Events {
					if evt.Key != m.ringKey() {
						continue
					}
					if evt.Type == etcdsim.EventTypeDelete {
						continue
					}
					var state ringState
					if err := json.Unmarshal([]byte(evt.Value), &state); err != nil {
						continue
					}
					if state.Nodes == nil {
						state.Nodes = map[string]NodeDescriptor{}
					}
					m.applyState(&state)
					assignments := make([]VirtualNodeAssignment, len(state.Tokens))
					copy(assignments, state.Tokens)
					select {
					case events <- RingEvent{Version: state.Version, Assignments: assignments}:
					case <-ctx.Done():
						return
					}
				}
			}
		}
	}()

	return events, nil
}

func (m *RingManager) applyState(state *ringState) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.state = *state
	m.rebuildLocked()
}

// Nodes returns all currently registered nodes.
func (m *RingManager) Nodes() []NodeDescriptor {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]NodeDescriptor, 0, len(m.state.Nodes))
	for _, node := range m.state.Nodes {
		out = append(out, node)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}
