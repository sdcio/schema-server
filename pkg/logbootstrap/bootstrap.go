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

package logbootstrap

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"

	"github.com/go-logr/logr"
	logf "github.com/sdcio/logger"
)

// Init configures JSON logging for standalone binaries (schema-server, schemac).
// Library use inside data-server should not call this; data-server sets DefaultLogger.
func Init(debug, trace bool) (logr.Logger, context.Context) {
	slogOpts := &slog.HandlerOptions{
		Level:       slog.LevelInfo,
		ReplaceAttr: logf.ReplaceTimeAttr,
	}
	if debug {
		slogOpts.Level = slog.Level(-logf.VDebug)
	}
	if trace {
		slogOpts.Level = slog.Level(-logf.VTrace)
	}

	var output io.Writer = os.Stdout
	if logFile := os.Getenv("EXTRA_LOG_FILE"); logFile != "" {
		f, err := os.OpenFile(logFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0666)
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to open log file %s: %v\n", logFile, err)
		} else {
			output = io.MultiWriter(os.Stdout, f)
		}
	}

	log := logr.FromSlogHandler(slog.NewJSONHandler(output, slogOpts))
	logf.SetDefaultLogger(log)
	ctx := logf.IntoContext(context.Background(), log)
	return log, ctx
}
