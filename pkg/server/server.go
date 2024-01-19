package server

import (
	"context"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/mux"
	grpc_middleware "github.com/grpc-ecosystem/go-grpc-middleware"
	grpc_prometheus "github.com/grpc-ecosystem/go-grpc-prometheus"
	sdcpb "github.com/iptecharch/sdc-protos/sdcpb"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	log "github.com/sirupsen/logrus"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	_ "google.golang.org/grpc/encoding/gzip" // Install the gzip compressor

	"github.com/iptecharch/schema-server/pkg/config"
	"github.com/iptecharch/schema-server/pkg/schema"
	"github.com/iptecharch/schema-server/pkg/store"
	"github.com/iptecharch/schema-server/pkg/store/memstore"
	"github.com/iptecharch/schema-server/pkg/store/persiststore"
)

type Server struct {
	config *config.Config

	cfn context.CancelFunc

	schemaStore store.Store

	srv *grpc.Server
	sdcpb.UnimplementedSchemaServerServer

	router *mux.Router
	reg    *prometheus.Registry
}

func NewServer(c *config.Config) (*Server, error) {
	ctx, cancel := context.WithCancel(context.TODO())
	var s = &Server{
		config: c,
		cfn:    cancel,
		router: mux.NewRouter(),
		reg:    prometheus.NewRegistry(),
	}

	switch c.SchemaStore.Type {
	case "persistent":
		var err error
		s.schemaStore, err = persiststore.New(ctx, c.SchemaStore.Path)
		if err != nil {
			return nil, err
		}
	default: // memory
		s.schemaStore = memstore.New()
	}
	ls, err := s.schemaStore.ListSchema(ctx, &sdcpb.ListSchemaRequest{})
	if err != nil {
		return nil, err
	}
	for _, storeSc := range ls.GetSchema() {
		log.Debugf("schema store has schema %s", storeSc.String())
	}
	// gRPC server options
	opts := []grpc.ServerOption{
		grpc.MaxRecvMsgSize(c.GRPCServer.MaxRecvMsgSize),
	}

	if c.Prometheus != nil {
		grpcClientMetrics := grpc_prometheus.NewClientMetrics()
		s.reg.MustRegister(grpcClientMetrics)

		// add gRPC server interceptors for the Schema/Data server
		grpcMetrics := grpc_prometheus.NewServerMetrics()
		opts = append(opts,
			grpc.StreamInterceptor(grpcMetrics.StreamServerInterceptor()),
		)
		unaryInterceptors := []grpc.UnaryServerInterceptor{
			func(ctx context.Context, req interface{}, _ *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (resp interface{}, err error) {
				ctx, cfn := context.WithTimeout(ctx, c.GRPCServer.RPCTimeout)
				defer cfn()
				return handler(ctx, req)
			},
		}
		unaryInterceptors = append(unaryInterceptors, grpcMetrics.UnaryServerInterceptor())
		opts = append(opts, grpc.UnaryInterceptor(grpc_middleware.ChainUnaryServer(unaryInterceptors...)))
		s.reg.MustRegister(grpcMetrics)
	}

	if c.GRPCServer.TLS != nil {
		tlsCfg, err := c.GRPCServer.TLS.NewConfig(ctx)
		if err != nil {
			return nil, err
		}
		opts = append(opts, grpc.Creds(credentials.NewTLS(tlsCfg)))
	}

	s.srv = grpc.NewServer(opts...)
	// parse schemas
	log.Infof("%d schema(s) configured...", len(c.SchemaStore.Schemas))
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
			if s.schemaStore.HasSchema(sck) {
				log.Infof("schema %s already exists in the store: not reloading it...", sck)
				return
			}
			sc, err := schema.NewSchema(sCfg)
			if err != nil {
				log.Errorf("schema %s parsing failed: %v", sCfg.Name, err)
				return
			}
			now := time.Now()
			err = s.schemaStore.AddSchema(sc)
			if err != nil {
				log.Errorf("failed to add schema %s: %v", sc.UniqueName(""), err)
				return
			}
			log.Infof("schema %s saved in %s", sc.UniqueName(""), time.Since(now))
		}(sCfg)
	}
	wg.Wait()
	// register Schema server gRPC Methods
	sdcpb.RegisterSchemaServerServer(s.srv, s)
	return s, nil
}

func (s *Server) Serve(ctx context.Context) error {
	l, err := net.Listen("tcp", s.config.GRPCServer.Address)
	if err != nil {
		return err
	}
	log.Infof("running server on %s", s.config.GRPCServer.Address)
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
		log.Errorf("HTTP server stopped: %v", err)
	}
}

func (s *Server) Stop() {
	s.srv.Stop()
	s.cfn()
}

func (s *Server) SchemaStore() store.Store {
	return s.schemaStore
}
