package pgxsim

import (
	"context"
	"errors"
	"sort"
	"sync"
)

// IsolationLevel enumerates transaction isolation levels supported by the simulator.
type IsolationLevel int

const (
	// IsoLevelSerializable emulates PostgreSQL's SERIALIZABLE isolation level.
	IsoLevelSerializable IsolationLevel = iota + 1
)

// TxAccessMode enumerates read/write behaviour.
type TxAccessMode int

const (
	// ReadOnly prevents writes.
	ReadOnly TxAccessMode = iota + 1
	// ReadWrite allows both reads and writes.
	ReadWrite
)

// TxOptions mirrors pgx.TxOptions.
type TxOptions struct {
	IsoLevel   IsolationLevel
	AccessMode TxAccessMode
}

// ErrSerialization is returned when the simulator detects a serialization anomaly.
var ErrSerialization = errors.New("pgxsim: serialization failure")

// ErrReadOnly is returned when a write is attempted on a read-only transaction.
var ErrReadOnly = errors.New("pgxsim: transaction is read-only")

// Record represents a metadata row in the simulated Postgres store.
type Record struct {
	Key        string
	Value      string
	Attributes map[string]string
	Version    int64
}

// Store backs the in-memory transactional system.
type Store struct {
	mu      sync.RWMutex
	entries map[string]Record
}

// NewStore constructs an empty store.
func NewStore() *Store {
	return &Store{entries: map[string]Record{}}
}

// Pool provides BeginTx semantics similar to pgxpool.Pool.
type Pool struct {
	store *Store
}

// NewPool constructs a Pool backed by the supplied store.
func NewPool(store *Store) *Pool {
	return &Pool{store: store}
}

// BeginTx starts a new transaction.
func (p *Pool) BeginTx(ctx context.Context, opts TxOptions) (*Tx, error) {
	if opts.IsoLevel != IsoLevelSerializable {
		return nil, errors.New("pgxsim: only serializable isolation supported")
	}
	return &Tx{
		store:     p.store,
		opts:      opts,
		writes:    map[string]Record{},
		deletes:   map[string]struct{}{},
		readset:   map[string]int64{},
		committed: false,
	}, nil
}

// Tx represents an in-flight serializable transaction.
type Tx struct {
	store *Store
	opts  TxOptions

	writes  map[string]Record
	deletes map[string]struct{}
	readset map[string]int64

	committed bool
	rolled    bool
	mu        sync.Mutex
}

func (tx *Tx) ensureWritable() error {
	if tx.opts.AccessMode == ReadOnly {
		return ErrReadOnly
	}
	return nil
}

// Get returns the record for the provided key.
func (tx *Tx) Get(ctx context.Context, key string) (Record, bool, error) {
	tx.mu.Lock()
	defer tx.mu.Unlock()
	if rec, ok := tx.writes[key]; ok {
		return cloneRecord(rec), true, nil
	}
	if _, deleted := tx.deletes[key]; deleted {
		return Record{}, false, nil
	}
	tx.store.mu.RLock()
	rec, ok := tx.store.entries[key]
	tx.store.mu.RUnlock()
	if ok {
		tx.readset[key] = rec.Version
		return cloneRecord(rec), true, nil
	}
	tx.readset[key] = 0
	return Record{}, false, nil
}

// Put upserts the given record.
func (tx *Tx) Put(ctx context.Context, rec Record) error {
	if err := tx.ensureWritable(); err != nil {
		return err
	}
	tx.mu.Lock()
	defer tx.mu.Unlock()
	tx.writes[rec.Key] = cloneRecord(rec)
	delete(tx.deletes, rec.Key)
	return nil
}

// Delete removes the given key.
func (tx *Tx) Delete(ctx context.Context, key string) error {
	if err := tx.ensureWritable(); err != nil {
		return err
	}
	tx.mu.Lock()
	defer tx.mu.Unlock()
	delete(tx.writes, key)
	tx.deletes[key] = struct{}{}
	return nil
}

// List returns up to limit records that match the prefix, in lexical order.
func (tx *Tx) List(ctx context.Context, prefix string, limit int) ([]Record, error) {
	tx.mu.Lock()
	defer tx.mu.Unlock()
	tx.store.mu.RLock()
	items := make([]Record, 0, len(tx.store.entries))
	for k, v := range tx.store.entries {
		if prefix == "" || hasPrefix(k, prefix) {
			items = append(items, cloneRecord(v))
			tx.readset[k] = v.Version
		}
	}
	tx.store.mu.RUnlock()
	sort.Slice(items, func(i, j int) bool { return items[i].Key < items[j].Key })
	if limit > 0 && len(items) > limit {
		items = items[:limit]
	}
	return items, nil
}

// Commit applies the transaction, returning ErrSerialization if the read versions were invalidated.
func (tx *Tx) Commit(ctx context.Context) error {
	tx.mu.Lock()
	defer tx.mu.Unlock()
	if tx.committed || tx.rolled {
		return errors.New("pgxsim: transaction closed")
	}
	tx.store.mu.Lock()
	defer tx.store.mu.Unlock()
	for key, expectedVersion := range tx.readset {
		current, ok := tx.store.entries[key]
		if !ok {
			if expectedVersion != 0 {
				tx.rolled = true
				return ErrSerialization
			}
			continue
		}
		if current.Version != expectedVersion {
			tx.rolled = true
			return ErrSerialization
		}
	}
	for key := range tx.deletes {
		delete(tx.store.entries, key)
	}
	for key, rec := range tx.writes {
		tx.store.entries[key] = cloneRecord(rec)
	}
	tx.committed = true
	return nil
}

// Rollback marks the transaction as closed without applying changes.
func (tx *Tx) Rollback(ctx context.Context) error {
	tx.mu.Lock()
	defer tx.mu.Unlock()
	if tx.committed || tx.rolled {
		return nil
	}
	tx.rolled = true
	return nil
}

func hasPrefix(s, prefix string) bool {
	if len(prefix) == 0 {
		return true
	}
	if len(prefix) > len(s) {
		return false
	}
	return s[:len(prefix)] == prefix
}

func cloneRecord(in Record) Record {
	out := Record{
		Key:     in.Key,
		Value:   in.Value,
		Version: in.Version,
	}
	if len(in.Attributes) > 0 {
		out.Attributes = make(map[string]string, len(in.Attributes))
		for k, v := range in.Attributes {
			out.Attributes[k] = v
		}
	}
	return out
}
