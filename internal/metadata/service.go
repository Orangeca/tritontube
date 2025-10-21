package metadata

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"tritontube/internal/metadata/etcdsim"
	"tritontube/internal/metadata/pgxsim"
)

// Service implements the MetadataServiceServer gRPC API using serialisable pgx transactions
// combined with etcd transactional revision checks. The implementation is intentionally
// dependency-free (using in-repo simulators) so that it can be built in constrained CI environments.
type Service struct {
	UnimplementedMetadataServiceServer

	writePool *pgxsim.Pool
	readPool  *pgxsim.Pool
	etcd      *etcdsim.Client

	maxRetries int
	keyPrefix  string
	clock      func() time.Time
}

// ServiceConfig configures a new Service instance.
type ServiceConfig struct {
	WritePool  *pgxsim.Pool
	ReadPool   *pgxsim.Pool
	Etcd       *etcdsim.Client
	KeyPrefix  string
	MaxRetries int
}

// NewService constructs a new metadata Service.
func NewService(cfg ServiceConfig) (*Service, error) {
	if cfg.WritePool == nil {
		return nil, errors.New("metadata: write pool is required")
	}
	if cfg.ReadPool == nil {
		cfg.ReadPool = cfg.WritePool
	}
	if cfg.Etcd == nil {
		return nil, errors.New("metadata: etcd client is required")
	}
	maxRetries := cfg.MaxRetries
	if maxRetries <= 0 {
		maxRetries = 5
	}
	prefix := cfg.KeyPrefix
	if prefix == "" {
		prefix = "metadata/"
	}
	return &Service{
		writePool:  cfg.WritePool,
		readPool:   cfg.ReadPool,
		etcd:       cfg.Etcd,
		maxRetries: maxRetries,
		keyPrefix:  prefix,
		clock:      time.Now,
	}, nil
}

// PutMetadata performs a conditional upsert guarded by a SERIALIZABLE pgx transaction and an etcd revision compare.
func (s *Service) PutMetadata(ctx context.Context, req *PutMetadataRequest) (*PutMetadataResponse, error) {
	if req == nil || req.Item == nil {
		return nil, errors.New("metadata: item is required")
	}
	item := req.Item.Clone()
	if item.Key == "" {
		return nil, errors.New("metadata: key is required")
	}

	var newRec pgxsim.Record
	err := s.retry(ctx, func(ctx context.Context) error {
		tx, err := s.writePool.BeginTx(ctx, pgxsim.TxOptions{IsoLevel: pgxsim.IsoLevelSerializable, AccessMode: pgxsim.ReadWrite})
		if err != nil {
			return err
		}
		defer tx.Rollback(ctx) // nolint:errcheck

		existing, ok, err := tx.Get(ctx, item.Key)
		if err != nil {
			return err
		}
		if req.ExpectedVersion > 0 {
			if !ok {
				return fmt.Errorf("metadata: missing key %s", item.Key)
			}
			if existing.Version != req.ExpectedVersion {
				return fmt.Errorf("metadata: version mismatch for %s", item.Key)
			}
		}
		var version int64 = 1
		if ok {
			version = existing.Version + 1
		}
		newRec = pgxsim.Record{
			Key:        item.Key,
			Value:      item.Value,
			Attributes: cloneMap(item.Attributes),
			Version:    version,
		}
		if err := tx.Put(ctx, newRec); err != nil {
			return err
		}
		if err := tx.Commit(ctx); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	item.Version = newRec.Version
	encoded, err := json.Marshal(item)
	if err != nil {
		return nil, err
	}
	txn := s.etcd.Txn(ctx)
	if req.ExpectedEtcdRevision >= 0 {
		txn = txn.If(etcdsim.CompareModRevision(s.etcdKey(item.Key), etcdsim.CompareOpEqual, req.ExpectedEtcdRevision))
	}
	etcdResp, err := txn.Then(etcdsim.OpPut(s.etcdKey(item.Key), string(encoded))).Commit()
	if err != nil {
		return nil, err
	}
	if !etcdResp.Succeeded {
		return nil, fmt.Errorf("metadata: etcd revision conflict for %s", item.Key)
	}

	return &PutMetadataResponse{Item: item, EtcdRevision: etcdResp.Revision}, nil
}

// GetMetadata reads a metadata item using a read-only serializable transaction.
func (s *Service) GetMetadata(ctx context.Context, req *GetMetadataRequest) (*GetMetadataResponse, error) {
	if req == nil || req.Key == "" {
		return nil, errors.New("metadata: key is required")
	}
	tx, err := s.readPool.BeginTx(ctx, pgxsim.TxOptions{IsoLevel: pgxsim.IsoLevelSerializable, AccessMode: pgxsim.ReadOnly})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx) // nolint:errcheck

	rec, ok, err := tx.Get(ctx, req.Key)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, fmt.Errorf("metadata: key %s not found", req.Key)
	}
	_ = tx.Rollback(ctx)
	return &GetMetadataResponse{Item: recordToItem(rec)}, nil
}

// DeleteMetadata removes a record using pgx + etcd transactional guards.
func (s *Service) DeleteMetadata(ctx context.Context, req *DeleteMetadataRequest) (*DeleteMetadataResponse, error) {
	if req == nil || req.Key == "" {
		return nil, errors.New("metadata: key is required")
	}
	var deleted bool
	err := s.retry(ctx, func(ctx context.Context) error {
		tx, err := s.writePool.BeginTx(ctx, pgxsim.TxOptions{IsoLevel: pgxsim.IsoLevelSerializable, AccessMode: pgxsim.ReadWrite})
		if err != nil {
			return err
		}
		defer tx.Rollback(ctx) // nolint:errcheck

		rec, ok, err := tx.Get(ctx, req.Key)
		if err != nil {
			return err
		}
		if !ok {
			return fmt.Errorf("metadata: key %s not found", req.Key)
		}
		if req.ExpectedVersion > 0 && rec.Version != req.ExpectedVersion {
			return fmt.Errorf("metadata: version mismatch for %s", req.Key)
		}
		if err := tx.Delete(ctx, req.Key); err != nil {
			return err
		}
		if err := tx.Commit(ctx); err != nil {
			return err
		}
		deleted = true
		return nil
	})
	if err != nil {
		return nil, err
	}
	if !deleted {
		return nil, fmt.Errorf("metadata: delete lost for %s", req.Key)
	}
	txn := s.etcd.Txn(ctx)
	if req.ExpectedEtcdRevision >= 0 {
		txn = txn.If(etcdsim.CompareModRevision(s.etcdKey(req.Key), etcdsim.CompareOpEqual, req.ExpectedEtcdRevision))
	}
	resp, err := txn.Then(etcdsim.OpDelete(s.etcdKey(req.Key))).Commit()
	if err != nil {
		return nil, err
	}
	if !resp.Succeeded {
		return nil, fmt.Errorf("metadata: etcd revision conflict for %s", req.Key)
	}
	return &DeleteMetadataResponse{EtcdRevision: resp.Revision}, nil
}

// ListMetadata lists records lexicographically with pagination.
func (s *Service) ListMetadata(ctx context.Context, req *ListMetadataRequest) (*ListMetadataResponse, error) {
	limit := int(req.Limit)
	if limit <= 0 {
		limit = 100
	}
	tx, err := s.readPool.BeginTx(ctx, pgxsim.TxOptions{IsoLevel: pgxsim.IsoLevelSerializable, AccessMode: pgxsim.ReadOnly})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx) // nolint:errcheck

	prefix := req.Prefix
	items, err := tx.List(ctx, prefix, limit+1)
	if err != nil {
		return nil, err
	}
	startKey := req.PageToken
	if startKey != "" {
		filtered := make([]pgxsim.Record, 0, len(items))
		for _, rec := range items {
			if rec.Key > startKey {
				filtered = append(filtered, rec)
			}
		}
		items = filtered
	}
	nextToken := ""
	if len(items) > limit {
		nextToken = items[limit].Key
		items = items[:limit]
	}
	out := make([]*MetadataItem, 0, len(items))
	for _, rec := range items {
		out = append(out, recordToItem(rec))
	}
	return &ListMetadataResponse{Items: out, NextPageToken: nextToken}, nil
}

func (s *Service) retry(ctx context.Context, fn func(context.Context) error) error {
	var lastErr error
	for attempt := 0; attempt < s.maxRetries; attempt++ {
		if err := fn(ctx); err != nil {
			if errors.Is(err, pgxsim.ErrSerialization) {
				lastErr = err
				continue
			}
			return err
		}
		return nil
	}
	if lastErr != nil {
		return lastErr
	}
	return fmt.Errorf("metadata: exceeded retry budget (%d)", s.maxRetries)
}

func (s *Service) etcdKey(key string) string {
	return s.keyPrefix + key
}

func cloneMap(src map[string]string) map[string]string {
	if len(src) == 0 {
		return nil
	}
	out := make(map[string]string, len(src))
	for k, v := range src {
		out[k] = v
	}
	return out
}

func recordToItem(rec pgxsim.Record) *MetadataItem {
	return &MetadataItem{
		Key:        rec.Key,
		Value:      rec.Value,
		Attributes: cloneMap(rec.Attributes),
		Version:    rec.Version,
	}
}
