package graph

import (
	"context"
	"fmt"
	apipb "github.com/autom8ter/graphik/api"
	"github.com/autom8ter/graphik/vm"
	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes"
	grpc_middleware "github.com/grpc-ecosystem/go-grpc-middleware"
	grpc_auth "github.com/grpc-ecosystem/go-grpc-middleware/auth"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/structpb"
	"time"
)

const (
	authCtxKey   = "x-graphik-auth-ctx"
	identityType = "identity"
	idClaim      = "sub"
	methodCtxKey = "x-grpc-full-method"
)

type intercept struct {
	Method    string
	Identity  map[string]interface{}
	Timestamp int64
	Request   map[string]interface{}
	Timing    apipb.Timing
}

func (r *intercept) AsMap() map[string]interface{} {
	return map[string]interface{}{
		"method":    r.Method,
		"identity":  r.Identity,
		"request":   r.Request,
		"timestamp": r.Timestamp,
		"timing":    r.Timing,
	}
}

func (g *GraphStore) UnaryAuth() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		token, err := grpc_auth.AuthFromMD(ctx, "Bearer")
		if err != nil {
			return nil, err
		}
		payload, err := g.auth.VerifyJWT(token)
		if err != nil {
			return nil, status.Errorf(codes.Unauthenticated, err.Error())
		}
		if exp, ok := payload["exp"].(int64); ok {
			if exp < time.Now().Unix() {
				return nil, status.Errorf(codes.Unauthenticated, "token expired")
			}
		}
		if exp, ok := payload["exp"].(int); ok {
			if int64(exp) < time.Now().Unix() {
				return nil, status.Errorf(codes.Unauthenticated, "token expired")
			}
		}
		ctx, identity, err := g.NodeToContext(ctx, payload)
		if err != nil {
			return nil, status.Errorf(codes.Internal, err.Error())
		}
		ctx = metadata.AppendToOutgoingContext(ctx, "Authorization", fmt.Sprintf("Bearer %s", token))
		ctx = g.MethodToContext(ctx, info.FullMethod)
		now := time.Now()
		if len(g.authorizers) > 0 {
			intercept := &intercept{
				Method:    info.FullMethod,
				Identity:  identity.AsMap(),
				Timestamp: now.UnixNano(),
			}
			if val, ok := req.(apipb.Mapper); ok {
				intercept.Request = val.AsMap()
			}
			result, err := vm.Eval(g.authorizers, intercept)
			if err != nil {
				return nil, err
			}
			if !result {
				return nil, status.Error(codes.PermissionDenied, "request authorization = denied")
			}
		}
		if len(g.triggers) > 0 {
			a, err := ptypes.MarshalAny(req.(proto.Message))
			if err != nil {
				return nil, err
			}
			intercept := &apipb.Interception{
				Method:    info.FullMethod,
				Identity:  identity,
				Timestamp: now.UnixNano(),
				Request:   a,
				Timing:    apipb.Timing_BEFORE,
			}
			for _, trigger := range g.triggers {
				intercept, err = trigger.Mutate(ctx, intercept)
				if err != nil {
					return nil, err
				}
			}
			if err := ptypes.UnmarshalAny(intercept.Request, req.(proto.Message)); err != nil {
				return nil, status.Error(codes.Internal, fmt.Sprintf("trigger failure: %s", err.Error()))
			}
		}
		resp, err := handler(ctx, req)
		if err != nil {
			return resp, err
		}
		if len(g.triggers) > 0 {
			a, err := ptypes.MarshalAny(resp.(proto.Message))
			if err != nil {
				return nil, err
			}
			intercept := &apipb.Interception{
				Method:    info.FullMethod,
				Identity:  identity,
				Timestamp: now.UnixNano(),
				Request:   a,
				Timing:    apipb.Timing_AFTER,
			}
			for _, trigger := range g.triggers {
				intercept, err = trigger.Mutate(ctx, intercept)
				if err != nil {
					return nil, err
				}
			}
			if err := ptypes.UnmarshalAny(intercept.Request, resp.(proto.Message)); err != nil {
				return nil, status.Error(codes.Internal, fmt.Sprintf("trigger failure: %s", err.Error()))
			}
		}
		return resp, nil
	}
}

func (g *GraphStore) StreamAuth() grpc.StreamServerInterceptor {
	return func(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		token, err := grpc_auth.AuthFromMD(ss.Context(), "Bearer")
		if err != nil {
			return err
		}

		payload, err := g.auth.VerifyJWT(token)
		if err != nil {
			return status.Errorf(codes.Unauthenticated, err.Error())
		}
		if val, ok := payload["exp"].(int64); ok && val < time.Now().Unix() {
			return status.Errorf(codes.Unauthenticated, "token expired")
		}
		if val, ok := payload["exp"].(float64); ok && int64(val) < time.Now().Unix() {
			return status.Errorf(codes.Unauthenticated, "token expired")
		}
		ctx, identity, err := g.NodeToContext(ss.Context(), payload)
		if err != nil {
			return status.Errorf(codes.Internal, err.Error())
		}
		ctx = metadata.AppendToOutgoingContext(ctx, "Authorization", fmt.Sprintf("Bearer %s", token))
		ctx = g.MethodToContext(ss.Context(), info.FullMethod)
		now := time.Now()
		if len(g.authorizers) > 0 {
			intercept := &intercept{
				Method:    info.FullMethod,
				Identity:  identity.AsMap(),
				Timestamp: now.UnixNano(),
			}
			if val, ok := srv.(apipb.Mapper); ok {
				intercept.Request = val.AsMap()
			}
			result, err := vm.Eval(g.authorizers, intercept)
			if err != nil {
				return err
			}
			if !result {
				return status.Error(codes.PermissionDenied, "request authorization = denied")
			}
		}
		if len(g.triggers) > 0 {
			a, err := ptypes.MarshalAny(srv.(proto.Message))
			if err != nil {
				return err
			}
			intercept := &apipb.Interception{
				Method:    info.FullMethod,
				Identity:  identity,
				Timestamp: now.UnixNano(),
				Request:   a,
				Timing:    apipb.Timing_BEFORE,
			}
			for _, trigger := range g.triggers {
				intercept, err = trigger.Mutate(ctx, intercept)
				if err != nil {
					return err
				}
			}
			if err := ptypes.UnmarshalAny(intercept.Request, srv.(proto.Message)); err != nil {
				return status.Error(codes.Internal, fmt.Sprintf("trigger failure: %s", err.Error()))
			}
		}
		wrapped := grpc_middleware.WrapServerStream(ss)
		wrapped.WrappedContext = ctx

		return handler(srv, wrapped)
	}
}

func (a *GraphStore) NodeToContext(ctx context.Context, payload map[string]interface{}) (context.Context, *apipb.Node, error) {
	var err error
	n, err := a.GetNode(ctx, &apipb.Path{
		Gtype: identityType,
		Gid:   payload[idClaim].(string),
	})
	if err != nil || n == nil {
		strct, _ := structpb.NewStruct(payload)
		n, err = a.createIdentity(ctx, &apipb.NodeConstructor{
			Path: &apipb.Path{
				Gtype: identityType,
				Gid:   payload[idClaim].(string),
			},
			Attributes: strct,
		})
		if err != nil {
			return nil, nil, err
		}
	}
	if n == nil {
		panic("empty node")
	}
	return context.WithValue(ctx, authCtxKey, n), n, nil
}

func (s *GraphStore) NodeContext(ctx context.Context) *apipb.Node {
	val, ok := ctx.Value(authCtxKey).(*apipb.Node)
	if ok {
		return val
	}
	val2, ok := ctx.Value(authCtxKey).(apipb.Node)
	if ok {
		return &val2
	}
	return nil
}

func (r *GraphStore) MethodContext(ctx context.Context) string {
	val, ok := ctx.Value(methodCtxKey).(string)
	if ok {
		return val
	}
	return ""
}

func (r *GraphStore) MethodToContext(ctx context.Context, path string) context.Context {
	return context.WithValue(ctx, methodCtxKey, path)
}
