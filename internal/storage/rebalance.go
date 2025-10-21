package storage

import (
	"context"
	"fmt"
	"time"

	storagepb "tritontube/internal/storage/proto"
)

// MigrationExecutor performs the actual data movement associated with a rebalance plan.
type MigrationExecutor interface {
	ExecutePlan(ctx context.Context, plan *storagepb.RebalancePlan) error
}

// MigrationFunc adapts a plain function to the MigrationExecutor interface.
type MigrationFunc func(ctx context.Context, plan *storagepb.RebalancePlan) error

// ExecutePlan implements MigrationExecutor.
func (f MigrationFunc) ExecutePlan(ctx context.Context, plan *storagepb.RebalancePlan) error {
	if f == nil {
		return nil
	}
	return f(ctx, plan)
}

// Rebalancer watches ring changes and triggers data migrations.
type Rebalancer struct {
	Manager  *RingManager
	Executor MigrationExecutor
	Deadline time.Duration
}

// Run blocks until the context is cancelled, applying rebalance plans as changes are
// observed in etcd. The executor is invoked with a context that is automatically
// bounded by Deadline (default 5 seconds) to ensure migrations start promptly.
func (r *Rebalancer) Run(ctx context.Context) error {
	if r == nil || r.Manager == nil {
		return fmt.Errorf("storage: rebalancer requires a ring manager")
	}
	if r.Executor == nil {
		r.Executor = MigrationFunc(func(context.Context, *storagepb.RebalancePlan) error { return nil })
	}
	deadline := r.Deadline
	if deadline <= 0 {
		deadline = 5 * time.Second
	}

	events, err := r.Manager.Watch(ctx)
	if err != nil {
		return err
	}

	for {
		select {
		case <-ctx.Done():
			return nil
		case evt, ok := <-events:
			if !ok {
				return nil
			}
			plan := &storagepb.RebalancePlan{
				PlanId:      fmt.Sprintf("rebalance-%d", evt.Version),
				RingVersion: evt.Version,
			}
			for _, assignment := range evt.Assignments {
				plan.Assignments = append(plan.Assignments, &storagepb.VirtualNode{
					Id:          assignment.ID,
					Token:       assignment.Token,
					OwnerNodeId: assignment.NodeID,
				})
			}
			execCtx, cancel := context.WithTimeout(ctx, deadline)
			err := r.Executor.ExecutePlan(execCtx, plan)
			cancel()
			if err != nil {
				return err
			}
		}
	}
}
