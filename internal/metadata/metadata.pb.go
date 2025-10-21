// Code generated manually to emulate protoc output for offline builds.
// source: proto/metadata.proto

package metadata

import (
	"context"
	"errors"
	"fmt"

	grpc "tritontube/internal/metadata/grpcstub"
)

// MetadataItem mirrors metadata.v1.MetadataItem.
type MetadataItem struct {
	Key        string            `json:"key,omitempty"`
	Value      string            `json:"value,omitempty"`
	Attributes map[string]string `json:"attributes,omitempty"`
	Version    int64             `json:"version,omitempty"`
}

func (x *MetadataItem) GetAttributes() map[string]string {
	if x != nil && x.Attributes != nil {
		return x.Attributes
	}
	return map[string]string{}
}

func (x *MetadataItem) Clone() *MetadataItem {
	if x == nil {
		return nil
	}
	out := &MetadataItem{
		Key:     x.Key,
		Value:   x.Value,
		Version: x.Version,
	}
	if len(x.Attributes) > 0 {
		out.Attributes = make(map[string]string, len(x.Attributes))
		for k, v := range x.Attributes {
			out.Attributes[k] = v
		}
	}
	return out
}

// PutMetadataRequest mirrors metadata.v1.PutMetadataRequest.
type PutMetadataRequest struct {
	Item                 *MetadataItem `json:"item,omitempty"`
	ExpectedVersion      int64         `json:"expected_version,omitempty"`
	ExpectedEtcdRevision int64         `json:"expected_etcd_revision,omitempty"`
}

func (x *PutMetadataRequest) GetItem() *MetadataItem {
	if x != nil {
		return x.Item
	}
	return nil
}

// PutMetadataResponse mirrors metadata.v1.PutMetadataResponse.
type PutMetadataResponse struct {
	Item         *MetadataItem `json:"item,omitempty"`
	EtcdRevision int64         `json:"etcd_revision,omitempty"`
}

// GetMetadataRequest mirrors metadata.v1.GetMetadataRequest.
type GetMetadataRequest struct {
	Key string `json:"key,omitempty"`
}

// GetMetadataResponse mirrors metadata.v1.GetMetadataResponse.
type GetMetadataResponse struct {
	Item *MetadataItem `json:"item,omitempty"`
}

// DeleteMetadataRequest mirrors metadata.v1.DeleteMetadataRequest.
type DeleteMetadataRequest struct {
	Key                  string `json:"key,omitempty"`
	ExpectedVersion      int64  `json:"expected_version,omitempty"`
	ExpectedEtcdRevision int64  `json:"expected_etcd_revision,omitempty"`
}

// DeleteMetadataResponse mirrors metadata.v1.DeleteMetadataResponse.
type DeleteMetadataResponse struct {
	EtcdRevision int64 `json:"etcd_revision,omitempty"`
}

// ListMetadataRequest mirrors metadata.v1.ListMetadataRequest.
type ListMetadataRequest struct {
	Prefix    string `json:"prefix,omitempty"`
	Limit     int32  `json:"limit,omitempty"`
	PageToken string `json:"page_token,omitempty"`
}

// ListMetadataResponse mirrors metadata.v1.ListMetadataResponse.
type ListMetadataResponse struct {
	Items         []*MetadataItem `json:"items,omitempty"`
	NextPageToken string          `json:"next_page_token,omitempty"`
}

// MetadataServiceClient is the client API for MetadataService.
type MetadataServiceClient interface {
	PutMetadata(ctx context.Context, in *PutMetadataRequest, opts ...grpc.CallOption) (*PutMetadataResponse, error)
	GetMetadata(ctx context.Context, in *GetMetadataRequest, opts ...grpc.CallOption) (*GetMetadataResponse, error)
	DeleteMetadata(ctx context.Context, in *DeleteMetadataRequest, opts ...grpc.CallOption) (*DeleteMetadataResponse, error)
	ListMetadata(ctx context.Context, in *ListMetadataRequest, opts ...grpc.CallOption) (*ListMetadataResponse, error)
}

type metadataServiceClient struct {
	cc grpc.ClientConnInterface
}

// NewMetadataServiceClient constructs a client backed by cc.
func NewMetadataServiceClient(cc grpc.ClientConnInterface) MetadataServiceClient {
	return &metadataServiceClient{cc: cc}
}

func (c *metadataServiceClient) PutMetadata(ctx context.Context, in *PutMetadataRequest, opts ...grpc.CallOption) (*PutMetadataResponse, error) {
	out := new(PutMetadataResponse)
	if err := c.cc.Invoke(ctx, "/metadata.v1.MetadataService/PutMetadata", in, out, opts...); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *metadataServiceClient) GetMetadata(ctx context.Context, in *GetMetadataRequest, opts ...grpc.CallOption) (*GetMetadataResponse, error) {
	out := new(GetMetadataResponse)
	if err := c.cc.Invoke(ctx, "/metadata.v1.MetadataService/GetMetadata", in, out, opts...); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *metadataServiceClient) DeleteMetadata(ctx context.Context, in *DeleteMetadataRequest, opts ...grpc.CallOption) (*DeleteMetadataResponse, error) {
	out := new(DeleteMetadataResponse)
	if err := c.cc.Invoke(ctx, "/metadata.v1.MetadataService/DeleteMetadata", in, out, opts...); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *metadataServiceClient) ListMetadata(ctx context.Context, in *ListMetadataRequest, opts ...grpc.CallOption) (*ListMetadataResponse, error) {
	out := new(ListMetadataResponse)
	if err := c.cc.Invoke(ctx, "/metadata.v1.MetadataService/ListMetadata", in, out, opts...); err != nil {
		return nil, err
	}
	return out, nil
}

// MetadataServiceServer is the server API for MetadataService.
type MetadataServiceServer interface {
	PutMetadata(context.Context, *PutMetadataRequest) (*PutMetadataResponse, error)
	GetMetadata(context.Context, *GetMetadataRequest) (*GetMetadataResponse, error)
	DeleteMetadata(context.Context, *DeleteMetadataRequest) (*DeleteMetadataResponse, error)
	ListMetadata(context.Context, *ListMetadataRequest) (*ListMetadataResponse, error)
	mustEmbedUnimplementedMetadataServiceServer()
}

// UnimplementedMetadataServiceServer provides forward compatible defaults.
type UnimplementedMetadataServiceServer struct{}

func (UnimplementedMetadataServiceServer) PutMetadata(context.Context, *PutMetadataRequest) (*PutMetadataResponse, error) {
	return nil, errors.New("method PutMetadata not implemented")
}

func (UnimplementedMetadataServiceServer) GetMetadata(context.Context, *GetMetadataRequest) (*GetMetadataResponse, error) {
	return nil, errors.New("method GetMetadata not implemented")
}

func (UnimplementedMetadataServiceServer) DeleteMetadata(context.Context, *DeleteMetadataRequest) (*DeleteMetadataResponse, error) {
	return nil, errors.New("method DeleteMetadata not implemented")
}

func (UnimplementedMetadataServiceServer) ListMetadata(context.Context, *ListMetadataRequest) (*ListMetadataResponse, error) {
	return nil, errors.New("method ListMetadata not implemented")
}

func (UnimplementedMetadataServiceServer) mustEmbedUnimplementedMetadataServiceServer() {}

// UnsafeMetadataServiceServer may be embedded for forward compatibility but is discouraged.
type UnsafeMetadataServiceServer interface {
	mustEmbedUnimplementedMetadataServiceServer()
}

// RegisterMetadataServiceServer wires the server into the registrar.
func RegisterMetadataServiceServer(s grpc.ServiceRegistrar, srv MetadataServiceServer) {
	if srv == nil {
		panic("metadata: server is nil")
	}
	s.RegisterService(&MetadataService_ServiceDesc, srv)
}

// MetadataService_ServiceDesc describes the service for our lightweight gRPC stub.
var MetadataService_ServiceDesc = grpc.ServiceDesc{
	ServiceName: "metadata.v1.MetadataService",
	HandlerType: (*MetadataServiceServer)(nil),
	Methods: []grpc.MethodDesc{
		{
			MethodName: "PutMetadata",
			Handler:    _MetadataService_PutMetadata_Handler,
		},
		{
			MethodName: "GetMetadata",
			Handler:    _MetadataService_GetMetadata_Handler,
		},
		{
			MethodName: "DeleteMetadata",
			Handler:    _MetadataService_DeleteMetadata_Handler,
		},
		{
			MethodName: "ListMetadata",
			Handler:    _MetadataService_ListMetadata_Handler,
		},
	},
	Streams:  []grpc.StreamDesc{},
	Metadata: "proto/metadata.proto",
}

func _MetadataService_PutMetadata_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(PutMetadataRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(MetadataServiceServer).PutMetadata(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/metadata.v1.MetadataService/PutMetadata",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(MetadataServiceServer).PutMetadata(ctx, req.(*PutMetadataRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _MetadataService_GetMetadata_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(GetMetadataRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(MetadataServiceServer).GetMetadata(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/metadata.v1.MetadataService/GetMetadata",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(MetadataServiceServer).GetMetadata(ctx, req.(*GetMetadataRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _MetadataService_DeleteMetadata_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(DeleteMetadataRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(MetadataServiceServer).DeleteMetadata(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/metadata.v1.MetadataService/DeleteMetadata",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(MetadataServiceServer).DeleteMetadata(ctx, req.(*DeleteMetadataRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _MetadataService_ListMetadata_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(ListMetadataRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(MetadataServiceServer).ListMetadata(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/metadata.v1.MetadataService/ListMetadata",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(MetadataServiceServer).ListMetadata(ctx, req.(*ListMetadataRequest))
	}
	return interceptor(ctx, in, info, handler)
}

// EncodeMetadataItem serialises an item as a deterministic string for etcd storage.
func EncodeMetadataItem(item *MetadataItem) (string, error) {
	if item == nil {
		return "", fmt.Errorf("metadata: nil item")
	}
	attrs := make(map[string]string, len(item.Attributes))
	for k, v := range item.Attributes {
		attrs[k] = v
	}
	return fmt.Sprintf("%s\n%d\n%v", item.Value, item.Version, attrs), nil
}
