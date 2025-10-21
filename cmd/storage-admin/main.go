package main

import (
	"context"
	"encoding/json"
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"tritontube/internal/metadata/etcdsim"
	"tritontube/internal/storage"
	storagepb "tritontube/internal/storage/proto"
)

func main() {
	var prefix string
	var deadline time.Duration
	var statePath string
	flag.StringVar(&prefix, "prefix", "/storage/cluster", "etcd prefix used for the ring state")
	flag.DurationVar(&deadline, "deadline", 5*time.Second, "maximum time to start migrations after a change")
	flag.StringVar(&statePath, "state", "", "optional path to a ring state snapshot (JSON)")
	flag.Parse()

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	etcd, err := etcdsim.New(etcdsim.Config{})
	if err != nil {
		log.Fatalf("failed to create etcd client: %v", err)
	}
	if statePath != "" {
		data, err := os.ReadFile(statePath)
		if err != nil {
			log.Fatalf("failed to read state file: %v", err)
		}
		if _, err := etcd.Put(ctx, managerRingKey(prefix), string(data)); err != nil {
			log.Fatalf("failed to preload ring state: %v", err)
		}
	}

	manager, err := storage.NewRingManager(storage.RingManagerConfig{Etcd: etcd, Prefix: prefix})
	if err != nil {
		log.Fatalf("failed to create ring manager: %v", err)
	}

	executor := storage.MigrationFunc(func(ctx context.Context, plan *storagepb.RebalancePlan) error {
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		return encoder.Encode(plan)
	})

	rebalancer := storage.Rebalancer{Manager: manager, Executor: executor, Deadline: deadline}
	if err := rebalancer.Run(ctx); err != nil && ctx.Err() == nil {
		log.Fatalf("rebalance failed: %v", err)
	}
}

func managerRingKey(prefix string) string {
	if prefix == "" {
		prefix = "/storage/cluster"
	}
	return prefix + "/ring"
}
