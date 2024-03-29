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

package main

import (
	"context"
	"fmt"
	"strconv"
	"sync"
	"time"

	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
	"github.com/spf13/pflag"
	"golang.org/x/sync/semaphore"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

var addr string
var ds string
var conc int64
var numVlan int
var cleanup bool
var interfaces []string
var candidate string

func main() {
	pflag.StringVarP(&addr, "address", "a", "localhost:55000", "schema/data server address")
	pflag.StringVarP(&ds, "ds", "", "srl1", "datastore name")
	pflag.Int64VarP(&conc, "concurrency", "", 250, "max concurrent set requests")
	pflag.StringSliceVarP(&interfaces, "interface", "", []string{"ethernet-1/1"}, "list of interfaces to provision")
	pflag.IntVarP(&numVlan, "vlans", "", 10, "number of vlans to configure")
	pflag.BoolVarP(&cleanup, "cleanup", "", false, "cleanup after creation")
	pflag.StringVarP(&candidate, "candidate", "", "default", "candidate name")
	pflag.Parse()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cc, dataClient, err := createDataClient(ctx, addr)
	if err != nil {
		panic(err)
	}
	defer cc.Close()
	_, err = dataClient.CreateDataStore(ctx, &sdcpb.CreateDataStoreRequest{
		Name: ds,
		Datastore: &sdcpb.DataStore{
			Type: sdcpb.Type_CANDIDATE,
			Name: candidate,
		},
	})
	if err != nil {
		panic(err)
	}
	// _ = crDsRsp
	wg := sync.WaitGroup{}
	wg.Add(numVlan * len(interfaces))
	sem := semaphore.NewWeighted(conc)
	now := time.Now()
	for _, iface := range interfaces {
		// loop, concurrent
		for i := 0; i < numVlan; i++ {
			err := sem.Acquire(ctx, 1)
			if err != nil {
				panic(err)
			}
			go func(iface string, i int) {
				defer wg.Done()
				defer sem.Release(1)
				index := strconv.Itoa(i)
				vlanID := strconv.Itoa(i + 1)
				setRsp, err := dataClient.SetData(ctx, &sdcpb.SetDataRequest{
					Name: ds,
					Datastore: &sdcpb.DataStore{
						Type: sdcpb.Type_CANDIDATE,
						Name: candidate,
					},
					Update: []*sdcpb.Update{
						// interface enable
						{
							Path: &sdcpb.Path{Elem: []*sdcpb.PathElem{
								{
									Name: "interface",
									Key: map[string]string{
										"name": iface,
									},
								},
								{
									Name: "admin-state",
								},
							}},
							Value: &sdcpb.TypedValue{
								Value: &sdcpb.TypedValue_StringVal{StringVal: "enable"},
							},
						},
						// interface vlan-tagging
						{
							Path: &sdcpb.Path{Elem: []*sdcpb.PathElem{
								{
									Name: "interface",
									Key: map[string]string{
										"name": iface,
									},
								},
								{
									Name: "vlan-tagging",
								},
							}},
							Value: &sdcpb.TypedValue{
								Value: &sdcpb.TypedValue_StringVal{StringVal: "true"},
							},
						},
						// interface description
						{
							Path: &sdcpb.Path{Elem: []*sdcpb.PathElem{
								{
									Name: "interface",
									Key: map[string]string{
										"name": iface,
									},
								},
								{
									Name: "description",
								},
							}},
							Value: &sdcpb.TypedValue{
								Value: &sdcpb.TypedValue_StringVal{StringVal: "if_desc"},
							},
						},
						// subinterface admin-state
						{
							Path: &sdcpb.Path{Elem: []*sdcpb.PathElem{
								{
									Name: "interface",
									Key: map[string]string{
										"name": iface,
									},
								},
								{
									Name: "subinterface",
									Key: map[string]string{
										"index": fmt.Sprintf("%d", i),
									},
								},
								{
									Name: "admin-state",
								},
							}},
							Value: &sdcpb.TypedValue{
								Value: &sdcpb.TypedValue_StringVal{StringVal: "enable"},
							},
						},
						// type bridged
						{
							Path: &sdcpb.Path{Elem: []*sdcpb.PathElem{
								{
									Name: "interface",
									Key: map[string]string{
										"name": iface,
									},
								},
								{
									Name: "subinterface",
									Key: map[string]string{
										"index": index,
									},
								},
								{
									Name: "type",
								},
							}},
							Value: &sdcpb.TypedValue{
								Value: &sdcpb.TypedValue_StringVal{StringVal: "bridged"},
							},
						},
						// subinterface description
						{
							Path: &sdcpb.Path{Elem: []*sdcpb.PathElem{
								{
									Name: "interface",
									Key: map[string]string{
										"name": iface,
									},
								},
								{
									Name: "subinterface",
									Key: map[string]string{
										"index": index,
									},
								},
								{
									Name: "description",
								},
							}},
							Value: &sdcpb.TypedValue{
								Value: &sdcpb.TypedValue_StringVal{StringVal: "subif_desc"},
							},
						},
						// subinterface vlan-id
						{
							Path: &sdcpb.Path{Elem: []*sdcpb.PathElem{
								{
									Name: "interface",
									Key: map[string]string{
										"name": iface,
									},
								},
								{
									Name: "subinterface",
									Key: map[string]string{
										"index": index,
									},
								},
								{
									Name: "vlan",
								},
								{
									Name: "encap",
								},
								{
									Name: "single-tagged",
								},
								{
									Name: "vlan-id",
								},
							}},
							Value: &sdcpb.TypedValue{
								Value: &sdcpb.TypedValue_StringVal{StringVal: vlanID},
							},
						},
					},
				})
				if err != nil {
					panic(err)
				}
				_ = setRsp
			}(iface, i)
		}
	}
	wg.Wait()
	fmt.Println("set requests done    :", time.Since(now))
	now = time.Now()
	commitRsp, err := dataClient.Commit(ctx, &sdcpb.CommitRequest{
		Name: ds,
		Datastore: &sdcpb.DataStore{
			Type: sdcpb.Type_CANDIDATE,
			Name: candidate,
		},
		Rebase: false,
		Stay:   false,
	})
	if err != nil {
		panic(err)
	}
	_ = commitRsp

	fmt.Println("commit ack after     :", time.Since(now))
	wg.Add(numVlan * len(interfaces))
	_, err = dataClient.CreateDataStore(ctx, &sdcpb.CreateDataStoreRequest{
		Name: ds,
		Datastore: &sdcpb.DataStore{
			Type: sdcpb.Type_CANDIDATE,
			Name: candidate,
		},
	})
	if err != nil {
		panic(err)
	}
	if !cleanup {
		return
	}
	fmt.Println("deleting")

	now = time.Now()
	for _, iface := range interfaces {
		for i := 0; i < numVlan; i++ {
			err := sem.Acquire(ctx, 1)
			if err != nil {
				panic(err)
			}
			go func(iface string, i int) {
				defer wg.Done()
				defer sem.Release(1)
				index := strconv.Itoa(i)
				setRsp, err := dataClient.SetData(ctx, &sdcpb.SetDataRequest{
					Name: ds,
					Datastore: &sdcpb.DataStore{
						Type: sdcpb.Type_CANDIDATE,
						Name: candidate,
					},
					Delete: []*sdcpb.Path{
						{Elem: []*sdcpb.PathElem{
							{
								Name: "interface",
								Key: map[string]string{
									"name": iface,
								},
							},
							{Name: "subinterface", Key: map[string]string{"index": index}},
						}},
					},
				},
				)
				if err != nil {
					panic(err)
				}
				_ = setRsp
			}(iface, i)
		}
	}
	wg.Wait()
	fmt.Println("delete requests done :", time.Since(now))
	now = time.Now()
	commitRsp, err = dataClient.Commit(ctx, &sdcpb.CommitRequest{
		Name: ds,
		Datastore: &sdcpb.DataStore{
			Type: sdcpb.Type_CANDIDATE,
			Name: candidate,
		},
		Rebase: false,
		Stay:   false,
	})
	if err != nil {
		panic(err)
	}
	_ = commitRsp
	fmt.Println("deletes commit ok    :", time.Since(now))
}

func createDataClient(ctx context.Context, addr string) (*grpc.ClientConn, sdcpb.DataServerClient, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	cc, err := grpc.DialContext(ctx, addr,
		grpc.WithBlock(),
		grpc.WithTransportCredentials(
			insecure.NewCredentials(),
		),
	)
	if err != nil {
		return nil, nil, err
	}
	return cc, sdcpb.NewDataServerClient(cc), nil
}
