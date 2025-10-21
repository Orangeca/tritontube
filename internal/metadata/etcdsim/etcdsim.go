package etcdsim

import (
	"context"
	"errors"
	"sync"
)

// Client simulates the minimal surface of go.etcd.io/etcd/client/v3 used by the metadata service.
type Client struct {
	mu       sync.Mutex
	revision int64
	kv       map[string]kvPair
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
	return &Client{kv: map[string]kvPair{}}, nil
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

	for _, op := range ops {
		switch op.typ {
		case opPut:
			t.client.revision++
			t.client.kv[op.key] = kvPair{value: op.value, modRevision: t.client.revision}
		case opDelete:
			if _, ok := t.client.kv[op.key]; ok {
				t.client.revision++
				delete(t.client.kv, op.key)
			}
		default:
			return nil, errors.New("etcdsim: unsupported op")
		}
	}

	return &TxnResponse{Succeeded: success, Revision: t.client.revision}, nil
}
