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
	"fmt"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/mux"
	grpc_middleware "github.com/grpc-ecosystem/go-grpc-middleware"
	grpc_prometheus "github.com/grpc-ecosystem/go-grpc-prometheus"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/sdcio/logger"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	_ "google.golang.org/grpc/encoding/gzip" // Install the gzip compressor

	"github.com/sdcio/schema-server/pkg/config"
	"github.com/sdcio/schema-server/pkg/schema"
	"github.com/sdcio/schema-server/pkg/store"
	"github.com/sdcio/schema-server/pkg/store/memstore"
	"github.com/sdcio/schema-server/pkg/store/persiststore"
)

type Server struct {
	config *config.Config

	logCtx context.Context
	cfn    context.CancelFunc

	schemaStore store.Store

	srv *grpc.Server
	sdcpb.UnimplementedSchemaServerServer

	router *mux.Router
	reg    *prometheus.Registry
}

func NewServer(ctx context.Context, c *config.Config) (*Server, error) {
	log := logger.FromContext(ctx)
	logCtx := ctx
	ctx, cancel := context.WithCancel(ctx)
	var s = &Server{
		config: c,
		logCtx: logCtx,
		cfn:    cancel,
		router: mux.NewRouter(),
		reg:    prometheus.NewRegistry(),
	}

	switch c.SchemaStore.Type {
	case config.StoreTypePersistent:
		var err error
		s.schemaStore, err = persiststore.New(ctx, c.SchemaStore.Path, c.SchemaStore.Cache, c.SchemaStore.ReadOnly)
		if err != nil {
			return nil, err
		}
	case config.StoreTypeMemory:
		s.schemaStore = memstore.New()
	default:
		return nil, fmt.Errorf("unknown schema store type %q", c.SchemaStore.Type)
	}
	ls, err := s.schemaStore.ListSchema(ctx, &sdcpb.ListSchemaRequest{})
	if err != nil {
		return nil, err
	}
	for _, storeSc := range ls.GetSchema() {
		if log.V(logger.VDebug).Enabled() {
			log.V(logger.VDebug).Info("schema store has schema", "schema", storeSc.String())
		}
	}
	opts := []grpc.ServerOption{
		grpc.MaxRecvMsgSize(c.GRPCServer.MaxRecvMsgSize),
	}

	unaryInterceptors := []grpc.UnaryServerInterceptor{
		timeoutUnaryInterceptor(c),
		contextLoggingUnaryInterceptor(logCtx),
	}
	streamInterceptors := []grpc.StreamServerInterceptor{
		contextLoggingStreamInterceptor(logCtx),
	}

	if c.Prometheus != nil {
		grpcClientMetrics := grpc_prometheus.NewClientMetrics()
		s.reg.MustRegister(grpcClientMetrics)

		grpcMetrics := grpc_prometheus.NewServerMetrics()
		streamInterceptors = append(streamInterceptors, grpcMetrics.StreamServerInterceptor())
		unaryInterceptors = append(unaryInterceptors, grpcMetrics.UnaryServerInterceptor())
		s.reg.MustRegister(grpcMetrics)
	}

	opts = append(opts,
		grpc.UnaryInterceptor(grpc_middleware.ChainUnaryServer(unaryInterceptors...)),
		grpc.StreamInterceptor(grpc_middleware.ChainStreamServer(streamInterceptors...)),
	)

	if c.GRPCServer.TLS != nil {
		tlsCfg, err := c.GRPCServer.TLS.NewConfig(ctx)
		if err != nil {
			return nil, err
		}
		opts = append(opts, grpc.Creds(credentials.NewTLS(tlsCfg)))
	}

	s.srv = grpc.NewServer(opts...)
	// parse schemas
	log.Info("parsing configured schemas", "count", len(c.SchemaStore.Schemas))
	wg := new(sync.WaitGroup)
	wg.Add(len(c.SchemaStore.Schemas))
	for _, sCfg := range c.SchemaStore.Schemas {
		go func(sCfg *config.SchemaConfig) {
			defer wg.Done()
			sck := store.SchemaKey{
				Name:    sCfg.Name,
				Vendor:  sCfg.Vendor,
				Version: sCfg.Version,
			}
			schemaLog := log.WithValues(
				"schema-name", sCfg.Name,
				"schema-vendor", sCfg.Vendor,
				"schema-version", sCfg.Version,
			)
			loadCtx := logger.IntoContext(ctx, schemaLog)
			if s.schemaStore.HasSchema(sck) {
				schemaLog.Info("schema already exists in the store, not reloading")
				return
			}
			sc, err := schema.NewSchema(loadCtx, sCfg)
			if err != nil {
				schemaLog.Error(err, "schema parsing failed")
				return
			}
			now := time.Now()
			err = s.schemaStore.AddSchema(sc)
			if err != nil {
				schemaLog.Error(err, "failed to add schema to store")
				return
			}
			schemaLog.Info("schema saved", "duration", time.Since(now).String())
		}(sCfg)
	}
	wg.Wait()
	// register Schema server gRPC Methods
	sdcpb.RegisterSchemaServerServer(s.srv, s)
	return s, nil
}

func (s *Server) Serve(ctx context.Context) error {
	log := logger.FromContext(ctx)
	l, err := net.Listen("tcp", s.config.GRPCServer.Address)
	if err != nil {
		return err
	}
	log.Info("running server", "address", s.config.GRPCServer.Address)
	if s.config.Prometheus != nil {
		go s.ServeHTTP()
	}
	err = s.srv.Serve(l)
	if err != nil {
		return err
	}

	return nil
}

func (s *Server) ServeHTTP() {
	s.router.Handle("/metrics", promhttp.HandlerFor(s.reg, promhttp.HandlerOpts{}))
	s.reg.MustRegister(collectors.NewGoCollector())
	s.reg.MustRegister(collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}))
	srv := &http.Server{
		Addr:         s.config.Prometheus.Address,
		Handler:      s.router,
		ReadTimeout:  time.Minute,
		WriteTimeout: time.Minute,
	}
	err := srv.ListenAndServe()
	if err != nil {
		logger.DefaultLogger.Error(err, "HTTP server stopped")
	}
}

func (s *Server) Stop() {
	s.srv.Stop()
	s.cfn()
}

func (s *Server) SchemaStore() store.Store {
	return s.schemaStore
}
