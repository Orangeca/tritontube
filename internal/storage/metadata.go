package storage

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"tritontube/internal/metadata/etcdsim"
	storagepb "tritontube/internal/storage/proto"
)

// SegmentRecord tracks the authoritative location for an uploaded DASH segment.
type SegmentRecord struct {
	SegmentID   string                   `json:"segment_id"`
	Locator     storagepb.SegmentLocator `json:"locator"`
	PrimaryNode string                   `json:"primary_node"`
	Replicas    []string                 `json:"replicas"`
	Checksum    string                   `json:"checksum"`
	SizeBytes   int64                    `json:"size_bytes"`
	Attributes  map[string]string        `json:"attributes"`
	UpdatedAt   time.Time                `json:"updated_at"`
}

// MetadataStore persists segment metadata after successful replication.
type MetadataStore interface {
	PutSegment(ctx context.Context, record SegmentRecord) error
}

// EtcdMetadataStore stores segment metadata in etcd under a configurable prefix.
type EtcdMetadataStore struct {
	etcd   *etcdsim.Client
	prefix string
}

// EtcdMetadataStoreConfig configures the metadata store.
type EtcdMetadataStoreConfig struct {
	Etcd   *etcdsim.Client
	Prefix string
}

// NewEtcdMetadataStore constructs a metadata store backed by etcd.
func NewEtcdMetadataStore(cfg EtcdMetadataStoreConfig) (*EtcdMetadataStore, error) {
	if cfg.Etcd == nil {
		return nil, errors.New("storage: etcd client is required for metadata store")
	}
	if cfg.Prefix == "" {
		cfg.Prefix = "/storage/segments"
	}
	return &EtcdMetadataStore{etcd: cfg.Etcd, prefix: cfg.Prefix}, nil
}

func (s *EtcdMetadataStore) key(segmentID string) string {
	return fmt.Sprintf("%s/%s", s.prefix, segmentID)
}

// PutSegment stores or updates segment metadata. The operation is idempotent and
// overwrites existing state.
func (s *EtcdMetadataStore) PutSegment(ctx context.Context, record SegmentRecord) error {
	if record.SegmentID == "" {
		return errors.New("storage: segment id is required")
	}
	record.UpdatedAt = time.Now().UTC()
	encoded, err := json.Marshal(record)
	if err != nil {
		return fmt.Errorf("storage: failed to encode metadata: %w", err)
	}
	_, err = s.etcd.Put(ctx, s.key(record.SegmentID), string(encoded))
	return err
}
