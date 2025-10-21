package storage

import (
	"context"
	"errors"
	"fmt"
	"sync"

	storagepb "tritontube/internal/storage/proto"
)

// ReplicationTransport abstracts the streaming RPC used to replicate a segment to
// another node. It enables the service to be tested without requiring a full gRPC stack.
type ReplicationTransport interface {
	ReplicateSegment(ctx context.Context, nodeID string, header *storagepb.UploadSegmentHeader, payload []byte) error
}

// NoopReplicationTransport is a drop-in transport used in tests or single node setups.
type NoopReplicationTransport struct{}

// ReplicateSegment implements the ReplicationTransport interface.
func (NoopReplicationTransport) ReplicateSegment(ctx context.Context, nodeID string, header *storagepb.UploadSegmentHeader, payload []byte) error {
	_ = ctx
	_ = nodeID
	_ = header
	_ = payload
	return nil
}

// InProcessReplicationTransport dispatches to handlers registered in memory. It is
// primarily useful for unit tests.
type InProcessReplicationTransport struct {
	mu       sync.RWMutex
	handlers map[string]ReplicaHandler
}

// ReplicaHandler handles replication requests for a given node.
type ReplicaHandler func(ctx context.Context, header *storagepb.UploadSegmentHeader, payload []byte) error

// NewInProcessReplicationTransport constructs a new transport.
func NewInProcessReplicationTransport() *InProcessReplicationTransport {
	return &InProcessReplicationTransport{handlers: map[string]ReplicaHandler{}}
}

// Register registers a handler for the given node ID.
func (t *InProcessReplicationTransport) Register(nodeID string, handler ReplicaHandler) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.handlers[nodeID] = handler
}

// ReplicateSegment dispatches to the registered handler.
func (t *InProcessReplicationTransport) ReplicateSegment(ctx context.Context, nodeID string, header *storagepb.UploadSegmentHeader, payload []byte) error {
	t.mu.RLock()
	handler, ok := t.handlers[nodeID]
	t.mu.RUnlock()
	if !ok {
		return fmt.Errorf("storage: no replication handler for node %s", nodeID)
	}
	return handler(ctx, header, payload)
}

// Ensure interface satisfaction at compile time.
var _ ReplicationTransport = NoopReplicationTransport{}
var _ ReplicationTransport = (*InProcessReplicationTransport)(nil)

// ErrReplicationFailed aggregates replication errors when multiple replicas fail.
type ErrReplicationFailed struct {
	Errors map[string]error
}

func (e ErrReplicationFailed) Error() string {
	if len(e.Errors) == 0 {
		return "replication failed"
	}
	return fmt.Sprintf("replication failed for %d replicas", len(e.Errors))
}

func (e ErrReplicationFailed) Unwrap() error {
	for _, err := range e.Errors {
		if err != nil {
			return err
		}
	}
	return nil
}

// MergeReplicationErrors aggregates individual errors into a single error value.
func MergeReplicationErrors(results map[string]error) error {
	failed := map[string]error{}
	for nodeID, err := range results {
		if err != nil {
			failed[nodeID] = err
		}
	}
	if len(failed) == 0 {
		return nil
	}
	return ErrReplicationFailed{Errors: failed}
}

// ValidateReplicationTargets ensures the replica set includes the primary.
func ValidateReplicationTargets(targets []string, primary string) error {
	if len(targets) == 0 {
		return errors.New("storage: replica set cannot be empty")
	}
	for _, t := range targets {
		if t == primary {
			return nil
		}
	}
	return fmt.Errorf("storage: replica set missing primary %s", primary)
}
