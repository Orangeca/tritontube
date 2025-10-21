package metadata

import (
	"context"
	"fmt"
	"testing"

	"tritontube/internal/metadata/etcdsim"
	"tritontube/internal/metadata/pgxsim"
)

func newTestService(t *testing.T) *Service {
	t.Helper()
	store := pgxsim.NewStore()
	writePool := pgxsim.NewPool(store)
	readPool := pgxsim.NewPool(store)
	etcd, err := etcdsim.New(etcdsim.Config{})
	if err != nil {
		t.Fatalf("failed to create etcd sim: %v", err)
	}
	svc, err := NewService(ServiceConfig{WritePool: writePool, ReadPool: readPool, Etcd: etcd})
	if err != nil {
		t.Fatalf("failed to create service: %v", err)
	}
	return svc
}

func TestPutGetLifecycle(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	const ingest = `{"status":"ingesting"}`
	putResp, err := svc.PutMetadata(ctx, &PutMetadataRequest{Item: &MetadataItem{Key: "video/1", Value: ingest}})
	if err != nil {
		t.Fatalf("put failed: %v", err)
	}
	if putResp.Item.Version != 1 {
		t.Fatalf("unexpected version %d", putResp.Item.Version)
	}

	getResp, err := svc.GetMetadata(ctx, &GetMetadataRequest{Key: "video/1"})
	if err != nil {
		t.Fatalf("get failed: %v", err)
	}
	if getResp.Item.Value != ingest {
		t.Fatalf("unexpected value %s", getResp.Item.Value)
	}

	// update with CAS semantics
	const ready = `{"status":"ready"}`
	updResp, err := svc.PutMetadata(ctx, &PutMetadataRequest{
		Item:                 &MetadataItem{Key: "video/1", Value: ready},
		ExpectedVersion:      putResp.Item.Version,
		ExpectedEtcdRevision: putResp.EtcdRevision,
	})
	if err != nil {
		t.Fatalf("update failed: %v", err)
	}
	if updResp.Item.Version != 2 {
		t.Fatalf("expected version 2 got %d", updResp.Item.Version)
	}

	_, err = svc.DeleteMetadata(ctx, &DeleteMetadataRequest{Key: "video/1", ExpectedVersion: updResp.Item.Version, ExpectedEtcdRevision: updResp.EtcdRevision})
	if err != nil {
		t.Fatalf("delete failed: %v", err)
	}

	if _, err := svc.GetMetadata(ctx, &GetMetadataRequest{Key: "video/1"}); err == nil {
		t.Fatalf("expected missing key after delete")
	}
}

func TestListPagination(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	for i := 0; i < 5; i++ {
		key := fmt.Sprintf("video/%d", i)
		if _, err := svc.PutMetadata(ctx, &PutMetadataRequest{Item: &MetadataItem{Key: key, Value: "{}"}}); err != nil {
			t.Fatalf("put %s failed: %v", key, err)
		}
	}

	resp, err := svc.ListMetadata(ctx, &ListMetadataRequest{Prefix: "video/", Limit: 2})
	if err != nil {
		t.Fatalf("list failed: %v", err)
	}
	if len(resp.Items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(resp.Items))
	}
	if resp.NextPageToken == "" {
		t.Fatalf("expected next page token")
	}

	resp2, err := svc.ListMetadata(ctx, &ListMetadataRequest{Prefix: "video/", Limit: 4, PageToken: resp.NextPageToken})
	if err != nil {
		t.Fatalf("list page2 failed: %v", err)
	}
	if len(resp2.Items) == 0 {
		t.Fatalf("expected more items on second page")
	}
}
