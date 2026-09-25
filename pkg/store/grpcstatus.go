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

package store

import (
	"errors"

	"github.com/sdcio/schema-server/pkg/schema"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// SchemaLookupError maps schema path resolution errors to gRPC status codes.
func SchemaLookupError(err error) error {
	if err == nil {
		return nil
	}
	var amb *schema.AmbiguousPathError
	if errors.As(err, &amb) {
		return status.Error(codes.FailedPrecondition, amb.Error())
	}
	return status.Error(codes.NotFound, err.Error())
}
