package etcdsim

import (
	"context"
	"errors"
	"strings"
	"sync"
)

// Client simulates the minimal surface of go.etcd.io/etcd/client/v3 used by the metadata service.
type Client struct {
	mu       sync.Mutex
	revision int64
	kv       map[string]kvPair
	watchers map[int64]*watchSubscription
	nextID   int64
}

type kvPair struct {
	value       string
	modRevision int64
}

// Config matches the structure of clientv3.Config for API compatibility.
type Config struct {
	Endpoints []string
}

// New constructs a new client using the provided config.
func New(config Config) (*Client, error) {
	_ = config
	return &Client{kv: map[string]kvPair{}, watchers: map[int64]*watchSubscription{}}, nil
}

// Close satisfies the client API.
func (c *Client) Close() error { return nil }

// Txn starts a new transaction builder.
func (c *Client) Txn(ctx context.Context) *Txn {
	return &Txn{client: c}
}

// CompareTarget enumerates supported compare targets.
type CompareTarget int

const (
	compareModRevision CompareTarget = iota + 1
)

// CompareOp enumerates supported operations.
type CompareOp string

const (
	// CompareOpEqual asserts equality.
	CompareOpEqual CompareOp = "="
)

// Cmp mirrors clientv3.Cmp.
type Cmp struct {
	key    string
	target CompareTarget
	op     CompareOp
	value  int64
}

// CompareModRevision constructs a comparison on the key's mod revision.
func CompareModRevision(key string, op CompareOp, value int64) Cmp {
	return Cmp{key: key, target: compareModRevision, op: op, value: value}
}

// Op represents a transactional mutation.
type Op struct {
	typ   opType
	key   string
	value string
}

type opType int

const (
	opPut opType = iota + 1
	opDelete
)

// OpPut stores the value.
func OpPut(key, value string) Op {
	return Op{typ: opPut, key: key, value: value}
}

// OpDelete removes a key.
func OpDelete(key string) Op {
	return Op{typ: opDelete, key: key}
}

// TxnResponse mirrors clientv3.TxnResponse.
type TxnResponse struct {
	Succeeded bool
	Revision  int64
}

// Txn encapsulates a conditional set of operations.
type Txn struct {
	client    *Client
	compares  []Cmp
	onSuccess []Op
	onFailure []Op
}

// If adds comparisons.
func (t *Txn) If(cmps ...Cmp) *Txn {
	t.compares = append(t.compares, cmps...)
	return t
}

// Then configures success operations.
func (t *Txn) Then(ops ...Op) *Txn {
	t.onSuccess = append(t.onSuccess, ops...)
	return t
}

// Else configures failure operations.
func (t *Txn) Else(ops ...Op) *Txn {
	t.onFailure = append(t.onFailure, ops...)
	return t
}

// Commit executes the transaction atomically.
func (t *Txn) Commit() (*TxnResponse, error) {
	t.client.mu.Lock()
	defer t.client.mu.Unlock()

	success := true
	for _, cmp := range t.compares {
		switch cmp.target {
		case compareModRevision:
			entry, ok := t.client.kv[cmp.key]
			switch cmp.op {
			case CompareOpEqual:
				var current int64
				if ok {
					current = entry.modRevision
				}
				if current != cmp.value {
					success = false
				}
			default:
				return nil, errors.New("etcdsim: unsupported compare op")
			}
		default:
			return nil, errors.New("etcdsim: unsupported compare target")
		}
		if !success {
			break
		}
	}

	ops := t.onSuccess
	if !success {
		ops = t.onFailure
	}

	if len(ops) == 0 {
		return &TxnResponse{Succeeded: success, Revision: t.client.revision}, nil
	}

	var events []WatchEvent
	for _, op := range ops {
		switch op.typ {
		case opPut:
			t.client.revision++
			t.client.kv[op.key] = kvPair{value: op.value, modRevision: t.client.revision}
			events = append(events, WatchEvent{Type: EventTypePut, Key: op.key, Value: op.value, ModRevision: t.client.revision})
		case opDelete:
			if _, ok := t.client.kv[op.key]; ok {
				t.client.revision++
				delete(t.client.kv, op.key)
				events = append(events, WatchEvent{Type: EventTypeDelete, Key: op.key, ModRevision: t.client.revision})
			}
		default:
			return nil, errors.New("etcdsim: unsupported op")
		}
	}

	if len(events) > 0 {
		t.client.notifyWatchersLocked(events)
	}

	return &TxnResponse{Succeeded: success, Revision: t.client.revision}, nil
}

// GetOption configures a Get operation.
type GetOption func(*getOptions)

type getOptions struct {
	prefix bool
}

// WithPrefix enables prefix matching.
func WithPrefix() GetOption {
	return func(o *getOptions) { o.prefix = true }
}

// KeyValue represents a value stored in etcd.
type KeyValue struct {
	Key         string
	Value       string
	ModRevision int64
}

// GetResponse mirrors the shape of clientv3.GetResponse for the supported fields.
type GetResponse struct {
	KVs      []KeyValue
	Revision int64
}

// Get returns the value(s) associated with the provided key.
func (c *Client) Get(ctx context.Context, key string, opts ...GetOption) (*GetResponse, error) {
	_ = ctx
	options := getOptions{}
	for _, opt := range opts {
		opt(&options)
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	resp := &GetResponse{Revision: c.revision}
	if options.prefix {
		for k, v := range c.kv {
			if strings.HasPrefix(k, key) {
				resp.KVs = append(resp.KVs, KeyValue{Key: k, Value: v.value, ModRevision: v.modRevision})
			}
		}
		return resp, nil
	}

	if v, ok := c.kv[key]; ok {
		resp.KVs = append(resp.KVs, KeyValue{Key: key, Value: v.value, ModRevision: v.modRevision})
	}
	return resp, nil
}

// Put writes a key outside of a transaction. This mirrors the convenience helper
// available in the real client.
func (c *Client) Put(ctx context.Context, key, value string) (*TxnResponse, error) {
	txn := c.Txn(ctx)
	txn = txn.Then(OpPut(key, value))
	return txn.Commit()
}

// Delete removes a key outside of a transaction.
func (c *Client) Delete(ctx context.Context, key string) (*TxnResponse, error) {
	txn := c.Txn(ctx)
	txn = txn.Then(OpDelete(key))
	return txn.Commit()
}

// EventType describes the mutation that occurred.
type EventType int

const (
	// EventTypePut indicates an insert/update.
	EventTypePut EventType = iota + 1
	// EventTypeDelete indicates a deletion.
	EventTypeDelete
)

// WatchEvent mirrors the shape of an etcd event.
type WatchEvent struct {
	Type        EventType
	Key         string
	Value       string
	ModRevision int64
}

// WatchResponse represents a batch of events.
type WatchResponse struct {
	Events   []WatchEvent
	Revision int64
}

type watchSubscription struct {
	prefix string
	ch     chan WatchResponse
	cancel context.CancelFunc
}

// Watch subscribes to updates on a prefix. It mirrors the behaviour of
// clientv3.Watcher for the subset of functionality required by the storage
// service. The returned channel is closed when the context is cancelled.
func (c *Client) Watch(ctx context.Context, prefix string) <-chan WatchResponse {
	ch := make(chan WatchResponse, 8)
	watchCtx, cancel := context.WithCancel(ctx)

	c.mu.Lock()
	c.nextID++
	id := c.nextID
	c.watchers[id] = &watchSubscription{prefix: prefix, ch: ch, cancel: cancel}
	c.mu.Unlock()

	go func() {
		<-watchCtx.Done()
		c.removeWatcher(id)
		close(ch)
	}()

	return ch
}

func (c *Client) removeWatcher(id int64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if w, ok := c.watchers[id]; ok {
		w.cancel()
		delete(c.watchers, id)
	}
}

func (c *Client) notifyWatchersLocked(events []WatchEvent) {
	if len(c.watchers) == 0 {
		return
	}
	for _, evt := range events {
		for id, watcher := range c.watchers {
			if watcher == nil {
				continue
			}
			if !strings.HasPrefix(evt.Key, watcher.prefix) {
				continue
			}
			select {
			case watcher.ch <- WatchResponse{Events: []WatchEvent{evt}, Revision: c.revision}:
			default:
				_ = id
			}
		}
	}
}
