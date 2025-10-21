package metadata

import (
	"context"
	"fmt"
	"testing"
	"time"

	"tritontube/internal/metadata/etcdsim"
	"tritontube/internal/metadata/pgxsim"
)

func BenchmarkPutMetadata(b *testing.B) {
	store := pgxsim.NewStore()
	writePool := pgxsim.NewPool(store)
	readPool := pgxsim.NewPool(store)
	etcd, _ := etcdsim.New(etcdsim.Config{})
	svc, _ := NewService(ServiceConfig{WritePool: writePool, ReadPool: readPool, Etcd: etcd, MaxRetries: 2})

	ctx := context.Background()
	for i := 0; i < 1000; i++ {
		key := fmt.Sprintf("video/%d", i)
		_, _ = svc.PutMetadata(ctx, &PutMetadataRequest{Item: &MetadataItem{Key: key, Value: "{}"}})
	}

	b.ResetTimer()
	b.ReportAllocs()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			key := fmt.Sprintf("bench/%d", i)
			_, err := svc.PutMetadata(ctx, &PutMetadataRequest{Item: &MetadataItem{Key: key, Value: "{}"}})
			if err != nil {
				b.Errorf("put failed: %v", err)
				return
			}
			i++
		}
	})
}

func BenchmarkGetMetadata(b *testing.B) {
	store := pgxsim.NewStore()
	writePool := pgxsim.NewPool(store)
	readPool := pgxsim.NewPool(store)
	etcd, _ := etcdsim.New(etcdsim.Config{})
	svc, _ := NewService(ServiceConfig{WritePool: writePool, ReadPool: readPool, Etcd: etcd})

	ctx := context.Background()
	for i := 0; i < 1000; i++ {
		key := fmt.Sprintf("video/%d", i)
		_, _ = svc.PutMetadata(ctx, &PutMetadataRequest{Item: &MetadataItem{Key: key, Value: "{}"}})
	}

	b.ResetTimer()
	b.ReportAllocs()
	b.RunParallel(func(pb *testing.PB) {
		idx := 0
		for pb.Next() {
			key := fmt.Sprintf("video/%d", idx%1000)
			if _, err := svc.GetMetadata(ctx, &GetMetadataRequest{Key: key}); err != nil {
				b.Errorf("get failed: %v", err)
				return
			}
			idx++
		}
	})
}

func BenchmarkListMetadata(b *testing.B) {
	store := pgxsim.NewStore()
	writePool := pgxsim.NewPool(store)
	readPool := pgxsim.NewPool(store)
	etcd, _ := etcdsim.New(etcdsim.Config{})
	svc, _ := NewService(ServiceConfig{WritePool: writePool, ReadPool: readPool, Etcd: etcd})

	ctx := context.Background()
	for i := 0; i < 5000; i++ {
		key := fmt.Sprintf("video/%d", i)
		_, _ = svc.PutMetadata(ctx, &PutMetadataRequest{Item: &MetadataItem{Key: key, Value: "{}"}})
	}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		start := time.Now()
		resp, err := svc.ListMetadata(ctx, &ListMetadataRequest{Prefix: "video/", Limit: 50})
		if err != nil {
			b.Fatalf("list failed: %v", err)
		}
		if len(resp.Items) == 0 {
			b.Fatalf("expected items")
		}
		if elapsed := time.Since(start); elapsed > 10*time.Millisecond {
			b.Fatalf("list exceeded latency budget: %s", elapsed)
		}
	}
}
