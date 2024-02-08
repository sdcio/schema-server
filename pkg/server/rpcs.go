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

	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
	log "github.com/sirupsen/logrus"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/sdcio/schema-server/pkg/config"
	"github.com/sdcio/schema-server/pkg/schema"
	"github.com/sdcio/schema-server/pkg/store"
)

func (s *Server) GetSchema(ctx context.Context, req *sdcpb.GetSchemaRequest) (*sdcpb.GetSchemaResponse, error) {
	log.Debugf("received GetSchemaRequest: %v", req)
	return s.schemaStore.GetSchema(ctx, req)
}

func (s *Server) ListSchema(ctx context.Context, req *sdcpb.ListSchemaRequest) (*sdcpb.ListSchemaResponse, error) {
	log.Debugf("received ListSchema: %v", req)
	return s.schemaStore.ListSchema(ctx, req)
}

func (s *Server) GetSchemaDetails(ctx context.Context, req *sdcpb.GetSchemaDetailsRequest) (*sdcpb.GetSchemaDetailsResponse, error) {
	log.Debugf("received GetSchemaDetails: %v", req)
	return s.schemaStore.GetSchemaDetails(ctx, req)
}

func (s *Server) CreateSchema(ctx context.Context, req *sdcpb.CreateSchemaRequest) (*sdcpb.CreateSchemaResponse, error) {
	log.Debugf("received CreateSchema: %v", req)
	return s.schemaStore.CreateSchema(ctx, req)
}

func (s *Server) ReloadSchema(ctx context.Context, req *sdcpb.ReloadSchemaRequest) (*sdcpb.ReloadSchemaResponse, error) {
	log.Debugf("received ReloadSchema: %v", req)
	return s.schemaStore.ReloadSchema(ctx, req)
}

func (s *Server) DeleteSchema(ctx context.Context, req *sdcpb.DeleteSchemaRequest) (*sdcpb.DeleteSchemaResponse, error) {
	log.Debugf("received DeleteSchema: %v", req)
	return s.schemaStore.DeleteSchema(ctx, req)
}

func (s *Server) ToPath(ctx context.Context, req *sdcpb.ToPathRequest) (*sdcpb.ToPathResponse, error) {
	log.Debugf("received ToPath: %v", req)
	return s.schemaStore.ToPath(ctx, req)
}

func (s *Server) ExpandPath(ctx context.Context, req *sdcpb.ExpandPathRequest) (*sdcpb.ExpandPathResponse, error) {
	log.Debugf("received ExpandPath: %v", req)
	return s.schemaStore.ExpandPath(ctx, req)
}

func (s *Server) UploadSchema(stream sdcpb.SchemaServer_UploadSchemaServer) error {
	log.Infof("starting upload stream")
	createReq, err := stream.Recv()
	if err != nil {
		return err
	}
	log.Debugf("received first msg in upload stream: %v", createReq)
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
		// case req.CreateSchema.GetSchema().GetName() == "":
		// 	return status.Error(codes.InvalidArgument, "missing schema name")
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
		log.Infof("uploading schema %s@%s@%s", scConfig.Name, scConfig.Vendor, scConfig.Version)
		if s.schemaStore.HasSchema(scKey) {
			log.Errorf("schema %s@%s@%s already exists", scConfig.Name, scConfig.Vendor, scConfig.Version)
			return status.Errorf(codes.InvalidArgument, "schema %s@%s@%s already exists", scConfig.Name, scConfig.Vendor, scConfig.Version)
		}
	}
	dirname := fmt.Sprintf("%s_%s_%s", scConfig.Name, scConfig.Vendor, scConfig.Version)
	err = os.RemoveAll(path.Join(s.config.GRPCServer.SchemaServer.SchemasDirectory, dirname))
	if err != nil {
		log.Errorf("failed to clean directory %s: %v", dirname, err)
		return status.Errorf(codes.Internal, "failed to clean directory %s: %v", dirname, err)
	}
	handledFiles := make(map[string]*os.File)
LOOP:
	for {
		updloadFileReq, err := stream.Recv()
		if err != nil {
			return err
		}
		log.Debugf("got upload msg file")
		switch updloadFileReq := updloadFileReq.Upload.(type) {
		case *sdcpb.UploadSchemaRequest_SchemaFile:
			log.Debugf("got upload msg file *sdcpb.UploadSchemaRequest_SchemaFile")
			if updloadFileReq.SchemaFile.GetFileName() == "" {
				return status.Error(codes.InvalidArgument, "missing file name")
			}
			var uplFile *os.File
			var ok bool
			fileName := path.Join(s.config.GRPCServer.SchemaServer.SchemasDirectory, dirname, updloadFileReq.SchemaFile.GetFileName())
			log.Debugf("creating file if it doesn't exist: %s", fileName)
			log.Debugf("handled files: %v", handledFiles)
			uplFile, ok = handledFiles[fileName]
			if !ok {
				log.Debugf("file doesn't exist %s, creating it", fileName)
				uplFile, err = createFileWithDir(fileName)
				if err != nil {
					return err
				}
				handledFiles[fileName] = uplFile
				log.Debugf("created file: %s", fileName)
			}

			if len(updloadFileReq.SchemaFile.GetContents()) > 0 {
				log.Debugf("writing %d to %s", len(updloadFileReq.SchemaFile.GetContents()), fileName)
				_, err = uplFile.Write(updloadFileReq.SchemaFile.GetContents())
				if err != nil {
					uplFile.Close()
					s.cleanSchemaDir(dirname)
					return err
				}
				log.Debugf("wrote %d to %s", len(updloadFileReq.SchemaFile.GetContents()), fileName)
			}
			if updloadFileReq.SchemaFile.GetHash() != nil {
				log.Debugf("got hash for file %s", fileName)
				var hash hash.Hash
				switch updloadFileReq.SchemaFile.GetHash().GetMethod() {
				case sdcpb.Hash_UNSPECIFIED:
					uplFile.Truncate(0)
					uplFile.Close()
					s.cleanSchemaDir(dirname)
					return status.Errorf(codes.InvalidArgument, "hash method unspecified")
				case sdcpb.Hash_MD5:
					hash = md5.New()
				case sdcpb.Hash_SHA256:
					hash = sha256.New()
				case sdcpb.Hash_SHA512:
					hash = sha512.New()
				}
				log.Debugf("reading file to calc hash %s", fileName)
				rb := make([]byte, 1024*1024)
				// rewind file
				_, err = uplFile.Seek(0, 0)
				if err != nil {
					uplFile.Close()
					s.cleanSchemaDir(dirname)
					return err
				}
				for {
					n, err := uplFile.Read(rb)
					if err != nil {
						if errors.Is(err, io.EOF) {
							break
						}
						uplFile.Close()
						s.cleanSchemaDir(dirname)
						return err
					}
					_, err = hash.Write(rb[:n])
					if err != nil {
						uplFile.Close()
						s.cleanSchemaDir(dirname)
						return err
					}
					rb = make([]byte, 1024*1024)
				}
				log.Debugf("calc hash %s", fileName)
				calcHash := hash.Sum(nil)
				log.Debugf("localhash %x: %s", calcHash, fileName)
				log.Debugf("rcvdhash %x: %s", updloadFileReq.SchemaFile.GetHash().GetHash(), fileName)
				if !bytes.Equal(calcHash, updloadFileReq.SchemaFile.GetHash().GetHash()) {
					uplFile.Close()
					s.cleanSchemaDir(dirname)
					return status.Errorf(codes.FailedPrecondition, "file %s has wrong hash", updloadFileReq.SchemaFile.GetFileName())
				}
				err = uplFile.Close()
				if err != nil {
					log.Errorf("failed to close file: %v", err)
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
			log.Debugf("got finalize msg")
			if len(handledFiles) != 0 {
				log.Errorf("got finalize but there are pending files")
				s.cleanSchemaDir(dirname)
				return status.Errorf(codes.FailedPrecondition, "not all files are fully uploaded")
			}
			break LOOP
		default:
			s.cleanSchemaDir(dirname)
			return status.Errorf(codes.InvalidArgument, "unexpected message type")
		}
	}
	log.Infof("all files uploaded, parsing schema...")

	sc, err := schema.NewSchema(scConfig)
	if err != nil {
		s.cleanSchemaDir(dirname)
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
	// Create the directory path
	dir := filepath.Dir(filePath)
	if err := os.MkdirAll(dir, os.ModePerm); err != nil {
		return nil, err
	}

	// Create the file
	return os.Create(filePath)
}

func (s *Server) cleanSchemaDir(dirname string) {
	err := os.RemoveAll(path.Join(s.config.GRPCServer.SchemaServer.SchemasDirectory, dirname))
	if err != nil {
		log.Errorf("failed to clean directory %s: %v", dirname, err)
	}
}
