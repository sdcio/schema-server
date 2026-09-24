// Copyright 2024 Nokia
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package server

import (
	"context"

	"github.com/google/uuid"
	grpc_middleware "github.com/grpc-ecosystem/go-grpc-middleware"
	"github.com/sdcio/logger"
	"github.com/sdcio/schema-server/pkg/config"
	"google.golang.org/grpc"
)

func timeoutUnaryInterceptor(c *config.Config) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, _ *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (resp interface{}, err error) {
		ctx, cfn := context.WithTimeout(ctx, c.GRPCServer.RPCTimeout)
		defer cfn()
		return handler(ctx, req)
	}
}

func contextLoggingUnaryInterceptor(logCtx context.Context) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, _ *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (resp interface{}, err error) {
		log := logger.FromContext(logCtx).WithValues("grpc-request-uuid", uuid.New().String())
		ctx = logger.IntoContext(ctx, log)
		return handler(ctx, req)
	}
}

func contextLoggingStreamInterceptor(logCtx context.Context) grpc.StreamServerInterceptor {
	return func(srv interface{}, ss grpc.ServerStream, _ *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		log := logger.FromContext(logCtx).WithValues("grpc-request-uuid", uuid.New().String())
		wss := grpc_middleware.WrapServerStream(ss)
		wss.WrappedContext = logger.IntoContext(wss.WrappedContext, log)
		return handler(srv, wss)
	}
}
