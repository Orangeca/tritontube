package storage

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"

	storagepb "tritontube/internal/storage/proto"
)

// Service implements the storage.v1.StorageService RPCs.
type Service struct {
	storagepb.UnimplementedStorageServiceServer

	nodeID            string
	ring              *RingManager
	fs                *FS
	s3                S3Uploader
	transport         ReplicationTransport
	metadata          MetadataStore
	replicationFactor int
	leaseTTL          time.Duration
}

// ServiceConfig configures a new storage service instance.
type ServiceConfig struct {
	NodeID            string
	Ring              *RingManager
	Filesystem        *FS
	S3                S3Uploader
	Transport         ReplicationTransport
	Metadata          MetadataStore
	ReplicationFactor int
	LeaseTTL          time.Duration
}

// NewService constructs a Service with sane defaults.
func NewService(cfg ServiceConfig) (*Service, error) {
	if cfg.NodeID == "" {
		return nil, errors.New("storage: node id is required")
	}
	if cfg.Ring == nil {
		return nil, errors.New("storage: ring manager is required")
	}
	if cfg.Filesystem == nil {
		return nil, errors.New("storage: filesystem is required")
	}
	svc := &Service{
		nodeID:            cfg.NodeID,
		ring:              cfg.Ring,
		fs:                cfg.Filesystem,
		s3:                cfg.S3,
		transport:         cfg.Transport,
		metadata:          cfg.Metadata,
		replicationFactor: cfg.ReplicationFactor,
		leaseTTL:          cfg.LeaseTTL,
	}
	if svc.replicationFactor <= 0 {
		svc.replicationFactor = 3
	}
	if svc.s3 == nil {
		svc.s3 = NoopS3Uploader{}
	}
	if svc.transport == nil {
		svc.transport = NoopReplicationTransport{}
	}
	if svc.leaseTTL <= 0 {
		svc.leaseTTL = 15 * time.Second
	}
	return svc, nil
}

// UploadSegment receives a client-streamed DASH segment, persists it locally, and
// asynchronously replicates to additional storage nodes and S3.
func (s *Service) UploadSegment(stream storagepb.StorageService_UploadSegmentServer) error {
	ctx := stream.Context()
	first, err := stream.Recv()
	if err != nil {
		return err
	}
	header := first.GetHeader()
	if header == nil {
		return errors.New("storage: upload stream missing header")
	}
	if header.Locator == nil {
		return errors.New("storage: upload header missing locator")
	}
	if header.SegmentId == "" {
		return errors.New("storage: segment id is required")
	}
	var payload bytes.Buffer
	for {
		msg, err := stream.Recv()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return err
		}
		if chunk := msg.GetChunk(); len(chunk) > 0 {
			if _, err := payload.Write(chunk); err != nil {
				return err
			}
		}
		if msg.GetCommit() {
			break
		}
	}

	data := payload.Bytes()
	size, checksum, err := s.fs.Put(header.Locator.Bucket, header.Locator.Object, bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("storage: failed to persist segment: %w", err)
	}

	replicaStatus := []*storagepb.ReplicaAck{{NodeId: s.nodeID, Success: true}}
	results := map[string]error{s.nodeID: nil}

	targets := s.ring.Lookup([]byte(header.SegmentId), s.replicationFactor)
	if len(targets) == 0 {
		targets = []string{s.nodeID}
	}
	if err := ValidateReplicationTargets(targets, s.nodeID); err != nil {
		return err
	}

	var wg sync.WaitGroup
	var mu sync.Mutex

	for _, nodeID := range targets {
		if nodeID == s.nodeID {
			continue
		}
		wg.Add(1)
		go func(target string) {
			defer wg.Done()
			err := s.transport.ReplicateSegment(ctx, target, header, data)
			mu.Lock()
			results[target] = err
			mu.Unlock()
		}(nodeID)
	}

	if header.S3Bucket != "" && header.S3Key != "" {
		wg.Add(1)
		go func(bucket, key string) {
			defer wg.Done()
			err := s.s3.UploadSegment(ctx, bucket, key, bytes.NewReader(data))
			mu.Lock()
			results[fmt.Sprintf("s3:%s/%s", bucket, key)] = err
			mu.Unlock()
		}(header.S3Bucket, header.S3Key)
	}

	wg.Wait()

	for nodeID, err := range results {
		if nodeID == s.nodeID {
			continue
		}
		ack := &storagepb.ReplicaAck{NodeId: nodeID, Success: err == nil}
		if err != nil {
			ack.ErrorMessage = err.Error()
		}
		replicaStatus = append(replicaStatus, ack)
	}

	aggregateErr := MergeReplicationErrors(results)
	if s.metadata != nil {
		if aggregateErr == nil {
			record := SegmentRecord{
				SegmentID:   header.SegmentId,
				Locator:     *header.Locator,
				PrimaryNode: s.nodeID,
				Checksum:    checksum,
				SizeBytes:   size,
				Attributes:  header.Attributes,
			}
			for _, nodeID := range targets {
				if nodeID == s.nodeID {
					continue
				}
				if err := results[nodeID]; err == nil {
					record.Replicas = append(record.Replicas, nodeID)
				}
			}
			if header.S3Bucket != "" && header.S3Key != "" {
				key := fmt.Sprintf("s3:%s/%s", header.S3Bucket, header.S3Key)
				if err := results[key]; err == nil {
					record.Replicas = append(record.Replicas, key)
				}
			}
			if err := s.metadata.PutSegment(ctx, record); err != nil {
				replicaStatus = append(replicaStatus, &storagepb.ReplicaAck{NodeId: "metadata", Success: false, ErrorMessage: err.Error()})
			} else {
				replicaStatus = append(replicaStatus, &storagepb.ReplicaAck{NodeId: "metadata", Success: true})
			}
		} else {
			replicaStatus = append(replicaStatus, &storagepb.ReplicaAck{NodeId: "metadata", Success: false, ErrorMessage: aggregateErr.Error()})
		}
	}

	if aggregateErr != nil {
		replicaStatus = append(replicaStatus, &storagepb.ReplicaAck{NodeId: "replication", Success: false, ErrorMessage: aggregateErr.Error()})
	}

	resp := &storagepb.UploadSegmentResponse{SizeCommitted: size, Checksum: checksum, ReplicaStatus: replicaStatus}
	if err := stream.SendAndClose(resp); err != nil {
		return err
	}
	return nil
}

// GetSegment streams a stored segment back to the caller.
func (s *Service) GetSegment(req *storagepb.GetSegmentRequest, stream storagepb.StorageService_GetSegmentServer) error {
	if req == nil || req.Locator == nil {
		return errors.New("storage: locator required")
	}
	reader, err := s.fs.Get(req.Locator.Bucket, req.Locator.Object)
	if err != nil {
		return err
	}
	defer reader.Close()

	buf := make([]byte, 128*1024)
	for {
		n, readErr := reader.Read(buf)
		if n > 0 {
			chunk := make([]byte, n)
			copy(chunk, buf[:n])
			if err := stream.Send(&storagepb.GetSegmentResponse{Chunk: chunk}); err != nil {
				return err
			}
		}
		if readErr != nil {
			if errors.Is(readErr, io.EOF) {
				return stream.Send(&storagepb.GetSegmentResponse{Eof: true})
			}
			return readErr
		}
	}
}

// Heartbeat records the node's availability and returns the current ring version.
func (s *Service) Heartbeat(ctx context.Context, req *storagepb.HeartbeatRequest) (*storagepb.HeartbeatResponse, error) {
	if req == nil {
		return nil, errors.New("storage: heartbeat request required")
	}
	if req.NodeId == "" {
		return nil, errors.New("storage: heartbeat missing node id")
	}
	descriptor := NodeDescriptor{
		ID:             req.NodeId,
		Address:        req.AdvertiseAddress,
		CapacityBytes:  req.CapacityBytes,
		AvailableBytes: req.AvailableBytes,
	}
	version, err := s.ring.UpsertNode(ctx, descriptor)
	if err != nil {
		return nil, err
	}
	return &storagepb.HeartbeatResponse{
		LeaseTtlSeconds:  int64(s.leaseTTL.Seconds()),
		RequireRebalance: false,
		RingVersion:      version,
	}, nil
}

// Rebalance returns the most recent ring assignments. Callers can use this to orchestrate
// data migration when nodes are added or removed.
func (s *Service) Rebalance(ctx context.Context, req *storagepb.RebalanceRequest) (*storagepb.RebalanceResponse, error) {
	_ = req
	assignments, version := s.ring.Assignments()
	plan := &storagepb.RebalancePlan{
		PlanId:      fmt.Sprintf("plan-%d", time.Now().UnixNano()),
		RingVersion: version,
	}
	for _, assignment := range assignments {
		plan.Assignments = append(plan.Assignments, &storagepb.VirtualNode{
			Id:          assignment.ID,
			Token:       assignment.Token,
			OwnerNodeId: assignment.NodeID,
		})
	}
	return &storagepb.RebalanceResponse{Plan: plan}, nil
}
