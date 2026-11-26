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

package schema_test

import (
	"testing"

	"github.com/sdcio/schema-server/pkg/config"
	"github.com/sdcio/schema-server/pkg/schema"
)

func loadMutateOnWriteSchema(t *testing.T) *schema.Schema {
	t.Helper()
	sc, err := schema.NewSchema(&config.SchemaConfig{
		Name:    "mutate-on-write-test",
		Vendor:  "test",
		Version: "0.0.1",
		Files:   []string{"testdata/mutate-on-write"},
	})
	if err != nil {
		t.Fatalf("NewSchema: %v", err)
	}
	return sc
}

func assertIsMutateOnWrite(t *testing.T, sc *schema.Schema, path []string, want bool) {
	t.Helper()
	entry, err := sc.GetEntry(path)
	if err != nil {
		t.Fatalf("GetEntry(%v): %v", path, err)
	}
	elem, err := sc.SchemaElemFromYEntry(entry, false)
	if err != nil {
		t.Fatalf("SchemaElemFromYEntry(%v): %v", path, err)
	}
	if got := elem.IsMutateOnWrite(); got != want {
		t.Errorf("path %v: IsMutateOnWrite() = %v, want %v", path, got, want)
	}
}

func TestMutateOnWrite_SdcioExtAndOverlays(t *testing.T) {
	sc := loadMutateOnWriteSchema(t)

	tests := []struct {
		name string
		path []string
		want bool
	}{
		{
			name: "container returns false",
			path: []string{"system"},
			want: false,
		},
		{
			name: "direct sdcio-ext:mutate-on-write on leaf",
			path: []string{"system", "direct-secret"},
			want: true,
		},
		{
			name: "direct sdcio-ext:mutate-on-write on leaf-list",
			path: []string{"system", "direct-secret-list"},
			want: true,
		},
		{
			name: "deviate add overlay under custom import prefix on leaf",
			path: []string{"system", "overlay-secret"},
			want: true,
		},
		{
			name: "deviate add overlay under custom import prefix on leaf-list",
			path: []string{"system", "overlay-secret-list"},
			want: true,
		},
		{
			name: "plain string leaf returns false",
			path: []string{"system", "plain-description"},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertIsMutateOnWrite(t, sc, tt.path, tt.want)
		})
	}
}

func TestMutateOnWrite_VendorExtensionsAndTypedefs(t *testing.T) {
	sc := loadMutateOnWriteSchema(t)

	tests := []struct {
		name string
		path []string
		want bool
	}{
		// OpenConfig
		{name: "openconfig-extensions:openconfig-hashed-value leaf", path: []string{"system", "oc-hashed-ext"}, want: true},
		{name: "openconfig-extensions:openconfig-hashed-value leaf-list", path: []string{"system", "oc-hashed-ext-list"}, want: true},
		{name: "openconfig-aaa-types:crypt-password-type leaf", path: []string{"system", "oc-crypt-pw"}, want: true},
		{name: "openconfig-aaa-types:crypt-password-type leaf-list", path: []string{"system", "oc-crypt-pw-list"}, want: true},

		// IETF / IANA
		{name: "iana-crypt-hash:crypt-hash", path: []string{"system", "ietf-crypt-hash"}, want: true},

		// Nokia SR Linux
		{name: "srl_nokia-extensions:hash-algorithm inline", path: []string{"system", "srl-inline-hash-algo"}, want: true},
		{name: "srl_nokia-extensions:generated-class-name PasswordHash inline", path: []string{"system", "srl-inline-pw-hash"}, want: true},
		{name: "srl_nokia-extensions:generated-class-name SecretKey inline leaf", path: []string{"system", "srl-inline-secret-key"}, want: true},
		{name: "srl_nokia-extensions:generated-class-name SecretKey inline leaf-list", path: []string{"system", "srl-inline-secret-key-list"}, want: true},
		{name: "srl_nokia-common:crypt-password-type", path: []string{"system", "srl-crypt-pw"}, want: true},

		// Tail-f / ConfD
		{name: "tailf-common:md5-digest-string", path: []string{"system", "tailf-md5"}, want: true},
		{name: "tailf-common:sha-256-digest-string", path: []string{"system", "tailf-sha256"}, want: true},
		{name: "tailf-common:sha-512-digest-string", path: []string{"system", "tailf-sha512"}, want: true},
		{name: "tailf-common:des3-cbc-encrypted-string", path: []string{"system", "tailf-des3"}, want: true},
		{name: "tailf-common:aes-cfb-128-encrypted-string", path: []string{"system", "tailf-aes128"}, want: true},
		{name: "tailf-common:aes-256-cfb-128-encrypted-string", path: []string{"system", "tailf-aes256"}, want: true},

		// Cisco IOS-XR
		{name: "Cisco-IOS-XR-types:Proprietary-password", path: []string{"system", "xr-proprietary"}, want: true},
		{name: "Cisco-IOS-XR-types:Md5-password", path: []string{"system", "xr-md5"}, want: true},
		{name: "Cisco-IOS-XR-types:Type6-password", path: []string{"system", "xr-type6"}, want: true},
		{name: "Cisco-IOS-XR-types:Type8-password", path: []string{"system", "xr-type8"}, want: true},
		{name: "Cisco-IOS-XR-types:Type9-password", path: []string{"system", "xr-type9"}, want: true},
		{name: "Cisco-IOS-XR-types:Type10-password", path: []string{"system", "xr-type10"}, want: true},

		// Negative cases
		{name: "tailf-common:suppress-echo stays false", path: []string{"system", "tailf-suppress-echo-leaf"}, want: false},
		{name: "junos-common-types:unreadable typedef stays false", path: []string{"system", "junos-unreadable-leaf"}, want: false},
		{name: "junos-common-types:unreadable extension stays false", path: []string{"system", "junos-unreadable-ext-leaf"}, want: false},
		{name: "openconfig-types:routing-password stays false", path: []string{"system", "oc-routing-pw"}, want: false},
		{name: "srl_nokia-common:routing-password stays false despite internal SecretKey ext", path: []string{"system", "srl-routing-pw"}, want: false},
		{name: "srl_nokia-extensions:generated-class-name IpAddress stays false", path: []string{"system", "srl-ip-address"}, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertIsMutateOnWrite(t, sc, tt.path, tt.want)
		})
	}
}

func TestMutateOnWrite_TypedefChainsUnionsAndLeafrefs(t *testing.T) {
	sc := loadMutateOnWriteSchema(t)

	tests := []struct {
		name string
		path []string
		want bool
	}{
		{name: "multi-hop typedef chain to iana-crypt-hash", path: []string{"system", "chained-typedef-secret"}, want: true},
		{name: "multi-hop typedef chain to ext-annotated typedef", path: []string{"system", "chained-ext-typedef-secret"}, want: true},
		{name: "union with string and crypt-password-type member", path: []string{"system", "union-with-crypt-typedef"}, want: true},
		{name: "union with inline srl hash-algorithm member", path: []string{"system", "union-with-inline-ext"}, want: true},
		{name: "union defined via typedef with digest member", path: []string{"system", "union-via-typedef"}, want: true},
		{name: "union where all members are cleartext stays false", path: []string{"system", "union-all-cleartext"}, want: false},
		{name: "2-hop normalized leafref chain to mutate-on-write leaf", path: []string{"system", "leafref-hop2"}, want: true},
		{name: "leafref nested under choice/case to mutate-on-write leaf", path: []string{"system", "auth-holder", "choice-leafref-to-secret"}, want: true},
		{name: "leafref to cleartext routing-password stays false", path: []string{"system", "leafref-to-cleartext"}, want: false},
		{name: "cyclic leafref terminates cleanly as false", path: []string{"system", "cycle-a"}, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertIsMutateOnWrite(t, sc, tt.path, tt.want)
		})
	}
}

func TestMutateOnWrite_RealSRLinuxModels(t *testing.T) {
	sc, err := schema.NewSchema(&config.SchemaConfig{
		Name:        "SRL-Native",
		Vendor:      "Nokia",
		Version:     "22.11.1",
		Files:       []string{"testdata/srl-latest-yang-models/srl_nokia/models"},
		Directories: []string{"testdata/srl-latest-yang-models/ietf"},
		Excludes:    []string{".*tools.*"},
	})
	if err != nil {
		t.Fatalf("NewSchema(SRL): %v", err)
	}

	tests := []struct {
		name string
		path []string
		want bool
	}{
		{
			name: "SRL admin-user password (PasswordHash + hash-algorithm ar2)",
			path: []string{"system", "aaa", "authentication", "admin-user", "password"},
			want: true,
		},
		{
			name: "SRL keychain authentication-key (inline SecretKey)",
			path: []string{"system", "authentication", "keychain", "key", "authentication-key"},
			want: true,
		},
		{
			name: "SRL tacacs secret-key (srl-comm:routing-password cleartext)",
			path: []string{"system", "aaa", "server-group", "server", "tacacs", "secret-key"},
			want: false,
		},
		{
			name: "SRL interface description (plain string)",
			path: []string{"interface", "description"},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertIsMutateOnWrite(t, sc, tt.path, tt.want)
		})
	}
}
