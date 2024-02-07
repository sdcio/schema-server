// Copyright 2024 Nokia
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
package cmd

import (
	"context"
	"crypto/md5"
	"crypto/sha256"
	"crypto/sha512"
	"fmt"
	"hash"
	"io/fs"
	"os"
	"path/filepath"

	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

var uploadSize int
var hashMethod string

// schemaUploadCmd represents the upload command
var schemaUploadCmd = &cobra.Command{
	Use:          "upload",
	Short:        "upload schemas to the schema server",
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, _ []string) error {
		switch hashMethod {
		case "md5", "sha256", "sha512":
		default:
			return fmt.Errorf("unknown hash method %q: must be one of 'md5', 'sha256', 'sha512'", hashMethod)
		}
		ctx, cancel := context.WithCancel(cmd.Context())
		defer cancel()
		schemaClient, err := createSchemaClient(ctx, addr)
		if err != nil {
			return err
		}
		uploadClient, err := schemaClient.UploadSchema(ctx)
		if err != nil {
			return err
		}
		rcvrErrCh := make(chan error, 1)
		senderErrCh := make(chan error, 1)
		// start rcver
		go func() {
			defer close(rcvrErrCh)
			// wait for response or error
			m := &sdcpb.UploadSchemaResponse{}
			rcvrErrCh <- uploadClient.RecvMsg(m)
		}()
		// start sender
		go func() {
			defer close(senderErrCh)
			err = uploadClient.Send(&sdcpb.UploadSchemaRequest{
				Upload: &sdcpb.UploadSchemaRequest_CreateSchema{
					CreateSchema: &sdcpb.CreateSchemaRequest{
						Schema: &sdcpb.Schema{
							Name:    schemaName,
							Vendor:  schemaVendor,
							Version: schemaVersion,
						},
						Exclude: schemaExcludes,
					},
				},
			})
			if err != nil {
				senderErrCh <- err
			}
			// walk files and upload
			for _, schemaFile := range schemaFiles {
				err = filepath.Walk(schemaFile, uploadFileFn(uploadClient, sdcpb.UploadSchemaFile_MODULE))
				if err != nil {
					senderErrCh <- err
				}
			}
			// walk dir and upload
			for _, schemaFile := range schemaDirs {
				err = filepath.Walk(schemaFile, uploadFileFn(uploadClient, sdcpb.UploadSchemaFile_DEPENDENCY))
				if err != nil {
					senderErrCh <- err
				}
			}
			// send finalize
			err = uploadClient.Send(&sdcpb.UploadSchemaRequest{
				Upload: &sdcpb.UploadSchemaRequest_Finalize{
					Finalize: &sdcpb.UploadSchemaFinalize{},
				},
			})
			if err != nil {
				senderErrCh <- err
			}
		}()

		select {
		case <-ctx.Done():
			return ctx.Err()
		case err := <-senderErrCh:
			if err != nil {
				log.Errorf("sender error: %v", err)
			}
		}
		log.Infof("schema uploaded, waiting for schema parsing")
		err = <-rcvrErrCh
		if err != nil {
			return err
		}
		log.Infof("schema parsed.")
		return nil
	},
}

func init() {
	schemaCmd.AddCommand(schemaUploadCmd)
	schemaUploadCmd.Flags().StringArrayVarP(&schemaFiles, "file", "", []string{}, "path to file(s) containing a YANG module")
	schemaUploadCmd.Flags().StringArrayVarP(&schemaDirs, "dir", "", []string{}, "path to file(s) containing a YANG module dependency")
	schemaUploadCmd.Flags().StringArrayVarP(&schemaExcludes, "exclude", "", []string{}, "regex of modules names to be excluded")
	//
	schemaUploadCmd.Flags().IntVarP(&uploadSize, "size", "", 1024*100, "upload chunk size")
	schemaUploadCmd.Flags().StringVarP(&hashMethod, "hash", "", "md5", "hash method: md5, sha256 or sha512")
}

func uploadFileFn(uploadClient sdcpb.SchemaServer_UploadSchemaClient, ft sdcpb.UploadSchemaFile_FileType) func(path string, info fs.FileInfo, err error) error {
	return func(path string, info fs.FileInfo, err error) error {
		log.Debugf("found file %s", path)
		if err != nil {
			return err
		}
		// skip dirs and non .yang files
		if info.IsDir() || filepath.Ext(path) != ".yang" {
			return nil
		}
		log.Infof("reading file %s", path)
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		// calculate hash
		hashVal := calcHash(hashMethod, b)
		log.Infof("uploading file %s", path)
		// divide the file in chunks of $uploadSize
		for start := 0; start < len(b); start += uploadSize {
			end := start + uploadSize
			if end > len(b) {
				end = len(b)
			}
			// send the chunk
			chunk := b[start:end]
			err = uploadClient.Send(&sdcpb.UploadSchemaRequest{
				Upload: &sdcpb.UploadSchemaRequest_SchemaFile{
					SchemaFile: &sdcpb.UploadSchemaFile{
						FileName: path,
						FileType: ft,
						Contents: chunk,
					},
				},
			})
			if err != nil {
				return err
			}
		}
		log.Infof("sending file hash %s", path)
		err = uploadClient.Send(&sdcpb.UploadSchemaRequest{
			Upload: &sdcpb.UploadSchemaRequest_SchemaFile{
				SchemaFile: &sdcpb.UploadSchemaFile{
					FileName: path,
					FileType: ft,
					Hash: &sdcpb.Hash{
						Method: sdcpb.Hash_MD5,
						Hash:   hashVal,
					},
				},
			},
		})
		return err
	}
}

func calcHash(method string, b []byte) []byte {
	var hash hash.Hash
	switch method {
	case "md5":
		hash = md5.New()
	case "sha256":
		hash = sha256.New()
	case "sha512":
		hash = sha512.New()
	}
	hash.Write(b)
	return hash.Sum(nil)
}
