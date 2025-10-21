package grpcstub

import (
	"context"
	"errors"
	"fmt"
	"reflect"
)

// CallOption mirrors grpc.CallOption but is intentionally empty.
type CallOption interface{}

// ClientConnInterface mirrors grpc.ClientConnInterface.
type ClientConnInterface interface {
	Invoke(ctx context.Context, method string, args any, reply any, opts ...CallOption) error
}

// UnaryHandler is the handler invoked by interceptors.
type UnaryHandler func(ctx context.Context, req interface{}) (interface{}, error)

// UnaryServerInfo mirrors grpc.UnaryServerInfo.
type UnaryServerInfo struct {
	Server     interface{}
	FullMethod string
}

// UnaryServerInterceptor mirrors grpc.UnaryServerInterceptor.
type UnaryServerInterceptor func(ctx context.Context, req interface{}, info *UnaryServerInfo, handler UnaryHandler) (interface{}, error)

// MethodDesc mirrors grpc.MethodDesc.
type MethodDesc struct {
	MethodName string
	Handler    func(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor UnaryServerInterceptor) (interface{}, error)
}

// StreamDesc is present for API compatibility but unused.
type StreamDesc struct{}

// ServiceDesc mirrors grpc.ServiceDesc.
type ServiceDesc struct {
	ServiceName string
	HandlerType interface{}
	Methods     []MethodDesc
	Streams     []StreamDesc
	Metadata    interface{}
}

// ServiceRegistrar is the minimal interface required by the generated registration helper.
type ServiceRegistrar interface {
	RegisterService(*ServiceDesc, interface{})
}

// Server provides a lightweight in-process service registry that can be used in tests.
type Server struct {
	services map[string]*registeredService
}

type registeredService struct {
	desc *ServiceDesc
	impl interface{}
}

// NewServer constructs a server that can service Invoke calls via the InProcessConn.
func NewServer() *Server {
	return &Server{services: map[string]*registeredService{}}
}

// RegisterService implements the ServiceRegistrar interface.
func (s *Server) RegisterService(desc *ServiceDesc, impl interface{}) {
	if desc == nil {
		panic("grpcstub: nil service descriptor")
	}
	if impl == nil {
		panic("grpcstub: nil implementation")
	}
	s.services[desc.ServiceName] = &registeredService{desc: desc, impl: impl}
}

// NewInProcessConn exposes a ClientConnInterface that can directly dispatch to the registered services.
func (s *Server) NewInProcessConn() ClientConnInterface {
	return &inProcessConn{server: s}
}

type inProcessConn struct {
	server *Server
}

func (c *inProcessConn) Invoke(ctx context.Context, method string, args any, reply any, opts ...CallOption) error {
	if c.server == nil {
		return errors.New("grpcstub: connection closed")
	}
	srvName, mthName, ok := splitMethod(method)
	if !ok {
		return errors.New("grpcstub: malformed method")
	}
	srv, ok := c.server.services[srvName]
	if !ok {
		return fmt.Errorf("grpcstub: unknown service %s", srvName)
	}
	for _, md := range srv.desc.Methods {
		if md.MethodName == mthName {
			if md.Handler == nil {
				return errors.New("grpcstub: missing handler")
			}
			dec := func(target interface{}) error {
				if target == nil || args == nil {
					return nil
				}
				dstVal := reflect.ValueOf(target)
				srcVal := reflect.ValueOf(args)
				if dstVal.Kind() != reflect.Pointer || srcVal.Kind() != reflect.Pointer {
					return errors.New("grpcstub: arguments must be pointers")
				}
				if dstVal.Type() != srcVal.Type() {
					return fmt.Errorf("grpcstub: cannot decode %s into %s", srcVal.Type(), dstVal.Type())
				}
				dstVal.Elem().Set(srcVal.Elem())
				return nil
			}
			interceptor := func(ctx context.Context, req interface{}, info *UnaryServerInfo, handler UnaryHandler) (interface{}, error) {
				return handler(ctx, req)
			}
			out, err := md.Handler(srv.impl, ctx, dec, interceptor)
			if err != nil {
				return err
			}
			if reply == nil || out == nil {
				return nil
			}
			dstVal := reflect.ValueOf(reply)
			srcVal := reflect.ValueOf(out)
			if dstVal.Kind() != reflect.Pointer || srcVal.Kind() != reflect.Pointer {
				return errors.New("grpcstub: reply must be pointer")
			}
			if dstVal.Type() != srcVal.Type() {
				return fmt.Errorf("grpcstub: cannot assign %s into %s", srcVal.Type(), dstVal.Type())
			}
			dstVal.Elem().Set(srcVal.Elem())
			return nil
		}
	}
	return fmt.Errorf("grpcstub: unknown method %s", mthName)
}

func splitMethod(fullMethod string) (service string, method string, ok bool) {
	if len(fullMethod) == 0 || fullMethod[0] != '/' {
		return "", "", false
	}
	fullMethod = fullMethod[1:]
	for i := 0; i < len(fullMethod); i++ {
		if fullMethod[i] == '/' {
			return fullMethod[:i], fullMethod[i+1:], true
		}
	}
	return "", "", false
}
