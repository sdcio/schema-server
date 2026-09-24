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
	"bytes"
	"context"
	"crypto/md5"
	"crypto/sha256"
	"crypto/sha512"
	"errors"
	"fmt"
	"hash"
	"io"
	"os"
	"path"
	"path/filepath"

	"github.com/go-logr/logr"
	"github.com/sdcio/logger"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/sdcio/schema-server/pkg/config"
	"github.com/sdcio/schema-server/pkg/schema"
	"github.com/sdcio/schema-server/pkg/store"
)

func (s *Server) GetSchema(ctx context.Context, req *sdcpb.GetSchemaRequest) (*sdcpb.GetSchemaResponse, error) {
	logRPC(ctx, "GetSchema", "request", req)
	return s.schemaStore.GetSchema(ctx, req)
}

func (s *Server) ListSchema(ctx context.Context, req *sdcpb.ListSchemaRequest) (*sdcpb.ListSchemaResponse, error) {
	logRPC(ctx, "ListSchema", "request", req)
	return s.schemaStore.ListSchema(ctx, req)
}

func (s *Server) GetSchemaDetails(ctx context.Context, req *sdcpb.GetSchemaDetailsRequest) (*sdcpb.GetSchemaDetailsResponse, error) {
	logRPC(ctx, "GetSchemaDetails", "request", req)
	return s.schemaStore.GetSchemaDetails(ctx, req)
}

func (s *Server) CreateSchema(ctx context.Context, req *sdcpb.CreateSchemaRequest) (*sdcpb.CreateSchemaResponse, error) {
	logRPC(ctx, "CreateSchema", "request", req)
	return s.schemaStore.CreateSchema(ctx, req)
}

func (s *Server) ReloadSchema(ctx context.Context, req *sdcpb.ReloadSchemaRequest) (*sdcpb.ReloadSchemaResponse, error) {
	logRPC(ctx, "ReloadSchema", "request", req)
	return s.schemaStore.ReloadSchema(ctx, req)
}

func (s *Server) DeleteSchema(ctx context.Context, req *sdcpb.DeleteSchemaRequest) (*sdcpb.DeleteSchemaResponse, error) {
	logRPC(ctx, "DeleteSchema", "request", req)
	return s.schemaStore.DeleteSchema(ctx, req)
}

func (s *Server) ToPath(ctx context.Context, req *sdcpb.ToPathRequest) (*sdcpb.ToPathResponse, error) {
	logRPC(ctx, "ToPath", "request", req)
	return s.schemaStore.ToPath(ctx, req)
}

func (s *Server) ExpandPath(ctx context.Context, req *sdcpb.ExpandPathRequest) (*sdcpb.ExpandPathResponse, error) {
	logRPC(ctx, "ExpandPath", "request", req)
	return s.schemaStore.ExpandPath(ctx, req)
}

func logRPC(ctx context.Context, name string, key string, req interface{}) {
	log := logger.FromContext(ctx).WithName(name)
	if log.V(logger.VDebug).Enabled() {
		log.V(logger.VDebug).Info("received request", key, req)
	}
}

func (s *Server) UploadSchema(stream sdcpb.SchemaServer_UploadSchemaServer) error {
	ctx := stream.Context()
	log := logger.FromContext(ctx).WithName("UploadSchema")
	log.Info("starting upload stream")
	createReq, err := stream.Recv()
	if err != nil {
		return err
	}
	if log.V(logger.VDebug).Enabled() {
		log.V(logger.VDebug).Info("received first upload message", "message", createReq)
	}
	scConfig := &config.SchemaConfig{
		Files:       []string{},
		Directories: []string{},
		Excludes:    []string{},
	}
	switch req := createReq.Upload.(type) {
	default:
		return status.Error(codes.InvalidArgument, "unexpected msg type: expecting UploadSchemaRequest_CreateSchema")
	case *sdcpb.UploadSchemaRequest_CreateSchema:
		switch {
		case req.CreateSchema.GetSchema().GetVendor() == "":
			return status.Error(codes.InvalidArgument, "missing schema vendor")
		case req.CreateSchema.GetSchema().GetVersion() == "":
			return status.Error(codes.InvalidArgument, "missing schema version")
		}
		scConfig.Name = req.CreateSchema.GetSchema().GetName()
		scConfig.Vendor = req.CreateSchema.GetSchema().GetVendor()
		scConfig.Version = req.CreateSchema.GetSchema().GetVersion()
		scKey := store.SchemaKey{
			Name:    scConfig.Name,
			Vendor:  scConfig.Vendor,
			Version: scConfig.Version,
		}
		scConfig.Excludes = req.CreateSchema.Exclude
		log.Info("uploading schema",
			"name", scConfig.Name,
			"vendor", scConfig.Vendor,
			"version", scConfig.Version,
		)
		if s.schemaStore.HasSchema(scKey) {
			return status.Errorf(codes.InvalidArgument, "schema %s@%s@%s already exists", scConfig.Name, scConfig.Vendor, scConfig.Version)
		}
	}
	dirname := fmt.Sprintf("%s_%s_%s", scConfig.Name, scConfig.Vendor, scConfig.Version)
	err = os.RemoveAll(path.Join(s.config.GRPCServer.SchemaServer.SchemasDirectory, dirname))
	if err != nil {
		log.Error(err, "failed to clean directory", "dirname", dirname)
		return status.Errorf(codes.Internal, "failed to clean directory %s: %v", dirname, err)
	}
	handledFiles := make(map[string]*os.File)
LOOP:
	for {
		updloadFileReq, err := stream.Recv()
		if err != nil {
			return err
		}
		if log.V(logger.VDebug).Enabled() {
			log.V(logger.VDebug).Info("got upload message")
		}
		switch updloadFileReq := updloadFileReq.Upload.(type) {
		case *sdcpb.UploadSchemaRequest_SchemaFile:
			if updloadFileReq.SchemaFile.GetFileName() == "" {
				return status.Error(codes.InvalidArgument, "missing file name")
			}
			var uplFile *os.File
			var ok bool
			fileName := path.Join(s.config.GRPCServer.SchemaServer.SchemasDirectory, dirname, updloadFileReq.SchemaFile.GetFileName())
			uplFile, ok = handledFiles[fileName]
			if !ok {
				uplFile, err = createFileWithDir(fileName)
				if err != nil {
					return err
				}
				handledFiles[fileName] = uplFile
			}

			if len(updloadFileReq.SchemaFile.GetContents()) > 0 {
				_, err = uplFile.Write(updloadFileReq.SchemaFile.GetContents())
				if err != nil {
					uplFile.Close()
					s.cleanSchemaDir(log, dirname)
					return err
				}
			}
			if updloadFileReq.SchemaFile.GetHash() != nil {
				var hash hash.Hash
				switch updloadFileReq.SchemaFile.GetHash().GetMethod() {
				case sdcpb.Hash_UNSPECIFIED:
					uplFile.Truncate(0)
					uplFile.Close()
					s.cleanSchemaDir(log, dirname)
					return status.Errorf(codes.InvalidArgument, "hash method unspecified")
				case sdcpb.Hash_MD5:
					hash = md5.New()
				case sdcpb.Hash_SHA256:
					hash = sha256.New()
				case sdcpb.Hash_SHA512:
					hash = sha512.New()
				}
				rb := make([]byte, 1024*1024)
				_, err = uplFile.Seek(0, 0)
				if err != nil {
					uplFile.Close()
					s.cleanSchemaDir(log, dirname)
					return err
				}
				for {
					n, err := uplFile.Read(rb)
					if err != nil {
						if errors.Is(err, io.EOF) {
							break
						}
						uplFile.Close()
						s.cleanSchemaDir(log, dirname)
						return err
					}
					_, err = hash.Write(rb[:n])
					if err != nil {
						uplFile.Close()
						s.cleanSchemaDir(log, dirname)
						return err
					}
					rb = make([]byte, 1024*1024)
				}
				calcHash := hash.Sum(nil)
				if !bytes.Equal(calcHash, updloadFileReq.SchemaFile.GetHash().GetHash()) {
					uplFile.Close()
					s.cleanSchemaDir(log, dirname)
					return status.Errorf(codes.FailedPrecondition, "file %s has wrong hash", updloadFileReq.SchemaFile.GetFileName())
				}
				err = uplFile.Close()
				if err != nil {
					log.Error(err, "failed to close file")
				}
				switch updloadFileReq.SchemaFile.GetFileType() {
				case sdcpb.UploadSchemaFile_MODULE:
					scConfig.Files = append(scConfig.Files, fileName)
				case sdcpb.UploadSchemaFile_DEPENDENCY:
					scConfig.Directories = append(scConfig.Directories, fileName)
				}
				delete(handledFiles, fileName)
			}
		case *sdcpb.UploadSchemaRequest_Finalize:
			if len(handledFiles) != 0 {
				s.cleanSchemaDir(log, dirname)
				return status.Errorf(codes.FailedPrecondition, "not all files are fully uploaded")
			}
			break LOOP
		default:
			s.cleanSchemaDir(log, dirname)
			return status.Errorf(codes.InvalidArgument, "unexpected message type")
		}
	}
	log.Info("all files uploaded, parsing schema")

	parseCtx := logger.IntoContext(ctx, log.WithValues(
		"schema-name", scConfig.Name,
		"schema-vendor", scConfig.Vendor,
		"schema-version", scConfig.Version,
	))
	sc, err := schema.NewSchema(parseCtx, scConfig)
	if err != nil {
		s.cleanSchemaDir(log, dirname)
		return err
	}
	err = s.schemaStore.AddSchema(sc)
	if err != nil {
		return err
	}
	stream.SendAndClose(&sdcpb.UploadSchemaResponse{})
	return nil
}

func (s *Server) GetSchemaElements(req *sdcpb.GetSchemaRequest, stream sdcpb.SchemaServer_GetSchemaElementsServer) error {
	ctx := stream.Context()
	ch, err := s.schemaStore.GetSchemaElements(ctx, req)
	if err != nil {
		return err
	}
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case sce, ok := <-ch:
			if !ok {
				return nil
			}
			err = stream.Send(&sdcpb.GetSchemaResponse{
				Schema: sce,
			})
			if err != nil {
				return err
			}
		}
	}
}

func createFileWithDir(filePath string) (*os.File, error) {
	dir := filepath.Dir(filePath)
	if err := os.MkdirAll(dir, os.ModePerm); err != nil {
		return nil, err
	}
	return os.Create(filePath)
}

func (s *Server) cleanSchemaDir(log logr.Logger, dirname string) {
	err := os.RemoveAll(path.Join(s.config.GRPCServer.SchemaServer.SchemasDirectory, dirname))
	if err != nil {
		log.Error(err, "failed to clean directory", "dirname", dirname)
	}
}
