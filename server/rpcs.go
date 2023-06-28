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

	"github.com/iptecharch/schema-server/config"
	schemapb "github.com/iptecharch/schema-server/protos/schema_server"
	"github.com/iptecharch/schema-server/schema"
	log "github.com/sirupsen/logrus"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (s *Server) GetSchema(ctx context.Context, req *schemapb.GetSchemaRequest) (*schemapb.GetSchemaResponse, error) {
	log.Debugf("received GetSchemaRequest: %v", req)
	return s.schemaStore.GetSchema(ctx, req)
}

func (s *Server) ListSchema(ctx context.Context, req *schemapb.ListSchemaRequest) (*schemapb.ListSchemaResponse, error) {
	log.Debugf("received ListSchema: %v", req)
	return s.schemaStore.ListSchema(ctx, req)
}

func (s *Server) GetSchemaDetails(ctx context.Context, req *schemapb.GetSchemaDetailsRequest) (*schemapb.GetSchemaDetailsResponse, error) {
	log.Debugf("received GetSchemaDetails: %v", req)
	return s.schemaStore.GetSchemaDetails(ctx, req)
}

func (s *Server) CreateSchema(ctx context.Context, req *schemapb.CreateSchemaRequest) (*schemapb.CreateSchemaResponse, error) {
	log.Debugf("received CreateSchema: %v", req)
	return s.schemaStore.CreateSchema(ctx, req)
}

func (s *Server) ReloadSchema(ctx context.Context, req *schemapb.ReloadSchemaRequest) (*schemapb.ReloadSchemaResponse, error) {
	log.Debugf("received ReloadSchema: %v", req)
	return s.schemaStore.ReloadSchema(ctx, req)
}

func (s *Server) DeleteSchema(ctx context.Context, req *schemapb.DeleteSchemaRequest) (*schemapb.DeleteSchemaResponse, error) {
	log.Debugf("received DeleteSchema: %v", req)
	return s.schemaStore.DeleteSchema(ctx, req)
}

func (s *Server) ToPath(ctx context.Context, req *schemapb.ToPathRequest) (*schemapb.ToPathResponse, error) {
	log.Debugf("received ToPath: %v", req)
	return s.schemaStore.ToPath(ctx, req)
}

func (s *Server) ExpandPath(ctx context.Context, req *schemapb.ExpandPathRequest) (*schemapb.ExpandPathResponse, error) {
	log.Debugf("received ExpandPath: %v", req)
	return s.schemaStore.ExpandPath(ctx, req)
}

func (s *Server) UploadSchema(stream schemapb.SchemaServer_UploadSchemaServer) error {
	createReq, err := stream.Recv()
	if err != nil {
		return err
	}

	var name string
	var vendor string
	var version string
	var files []string
	var dirs []string

	switch req := createReq.Upload.(type) {
	case *schemapb.UploadSchemaRequest_CreateSchema:
		switch {
		case req.CreateSchema.GetSchema().GetName() == "":
			return status.Error(codes.InvalidArgument, "missing schema name")
		case req.CreateSchema.GetSchema().GetVendor() == "":
			return status.Error(codes.InvalidArgument, "missing schema vendor")
		case req.CreateSchema.GetSchema().GetVersion() == "":
			return status.Error(codes.InvalidArgument, "missing schema version")
		}
		name = req.CreateSchema.GetSchema().GetName()
		vendor = req.CreateSchema.GetSchema().GetVendor()
		version = req.CreateSchema.GetSchema().GetVersion()
		scKey := schema.SchemaKey{
			Name:    name,
			Vendor:  vendor,
			Version: version,
		}
		if s.schemaStore.HasSchema(scKey) {
			return status.Errorf(codes.InvalidArgument, "schema %s/%s/%s already exists", name, vendor, version)
		}
	}
	dirname := fmt.Sprintf("%s_%s_%s", name, vendor, version)
	handledFiles := make(map[string]*os.File)
LOOP:
	for {
		updloadFileReq, err := stream.Recv()
		if err != nil {
			return err
		}
		switch updloadFileReq := updloadFileReq.Upload.(type) {
		case *schemapb.UploadSchemaRequest_SchemaFile:
			if updloadFileReq.SchemaFile.GetFileName() == "" {
				return status.Error(codes.InvalidArgument, "missing file name")
			}
			var uplFile *os.File
			var ok bool
			fileName := path.Join(s.config.GRPCServer.SchemaServer.SchemasDirectory, dirname, updloadFileReq.SchemaFile.GetFileName())
			if uplFile, ok = handledFiles[fileName]; !ok {
				osf, err := os.Create(fileName)
				if err != nil {
					return err
				}
				handledFiles[fileName] = osf
			}
			if len(updloadFileReq.SchemaFile.GetContents()) > 0 {
				_, err = uplFile.Write(updloadFileReq.SchemaFile.GetContents())
				if err != nil {
					uplFile.Truncate(0)
					uplFile.Close()
					os.Remove(fileName)
					return err
				}
			}
			if updloadFileReq.SchemaFile.GetHash() != nil {
				var hash hash.Hash
				switch updloadFileReq.SchemaFile.GetHash().GetMethod() {
				case schemapb.Hash_UNSPECIFIED:
					uplFile.Truncate(0)
					uplFile.Close()
					os.Remove(fileName)
					return status.Errorf(codes.InvalidArgument, "hash method unspecified")
				case schemapb.Hash_MD5:
					hash = md5.New()
				case schemapb.Hash_SHA256:
					hash = sha256.New()
				case schemapb.Hash_SHA512:
					hash = sha512.New()
				}
				rb := make([]byte, 1024*1024)
				for {
					n, err := uplFile.Read(rb)
					if err != nil {
						if errors.Is(err, io.EOF) {
							break
						}
						uplFile.Close()
						err2 := os.RemoveAll(path.Join(s.config.GRPCServer.SchemaServer.SchemasDirectory, dirname))
						if err2 != nil {
							log.Errorf("failed to delete %s: %v", dirname, err2)
						}
						return err
					}
					_, err = hash.Write(rb[:n])
					if err != nil {
						uplFile.Close()
						err2 := os.RemoveAll(path.Join(s.config.GRPCServer.SchemaServer.SchemasDirectory, dirname))
						if err2 != nil {
							log.Errorf("failed to delete %s: %v", dirname, err2)
						}
						return err
					}
					rb = make([]byte, 1024*1024)
				}
				calcHash := hash.Sum(nil)
				if !bytes.Equal(calcHash, updloadFileReq.SchemaFile.GetHash().GetHash()) {
					uplFile.Close()
					err2 := os.RemoveAll(path.Join(s.config.GRPCServer.SchemaServer.SchemasDirectory, dirname))
					if err2 != nil {
						log.Errorf("failed to delete %s: %v", dirname, err2)
					}
					return status.Errorf(codes.FailedPrecondition, "file %s has wrong hash", updloadFileReq.SchemaFile.GetFileName())
				}
				uplFile.Close()
				switch updloadFileReq.SchemaFile.GetFileType() {
				case schemapb.UploadSchemaFile_MODULE:
					files = append(files, fileName)
				case schemapb.UploadSchemaFile_DEPENDENCY:
					dirs = append(dirs, fileName)
				}
				delete(handledFiles, fileName)
			}
		case *schemapb.UploadSchemaRequest_Finalize:
			if len(handledFiles) != 0 {
				err2 := os.RemoveAll(path.Join(s.config.GRPCServer.SchemaServer.SchemasDirectory, dirname))
				if err2 != nil {
					log.Errorf("failed to delete %s: %v", dirname, err2)
				}
				return status.Errorf(codes.FailedPrecondition, "not all files are fully uploaded")
			}
			break LOOP
		default:
			err2 := os.RemoveAll(path.Join(s.config.GRPCServer.SchemaServer.SchemasDirectory, dirname))
			if err2 != nil {
				log.Errorf("failed to delete %s: %v", dirname, err2)
			}
			return status.Errorf(codes.InvalidArgument, "unexpected message type")
		}
	}

	sc, err := schema.NewSchema(
		&config.SchemaConfig{
			Name:        name,
			Vendor:      vendor,
			Version:     version,
			Files:       files,
			Directories: dirs,
		},
	)
	if err != nil {
		err2 := os.RemoveAll(path.Join(s.config.GRPCServer.SchemaServer.SchemasDirectory, dirname))
		if err2 != nil {
			log.Errorf("failed to delete %s: %v", dirname, err2)
		}
		return err
	}
	s.schemaStore.AddSchema(sc)
	return nil
}

func (s *Server) GetSchemaElements(req *schemapb.GetSchemaRequest, stream schemapb.SchemaServer_GetSchemaElementsServer) error {
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
			err = stream.Send(&schemapb.GetSchemaResponse{
				Schema: sce,
			})
			if err != nil {
				return err
			}
		}
	}
}
