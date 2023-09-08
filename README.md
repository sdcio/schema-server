# SCHEMA-SERVER

## build

```shell
make build
```

## run the server

```shell
./bin/schema-server
```

## run the client

### srl

```shell
# schema
version=23.3.2
bin/schemac schema get --name srl --version $version --vendor Nokia --path "/interface[name=ethernet-1/1]/subinterface"
bin/schemac schema get --name srl --version $version --vendor Nokia --path "/interface[name=ethernet-1/1]/subinterface" --all
bin/schemac schema get --name srl --version $version --vendor Nokia --path "/acl/cpm-filter/ipv4-filter/entry/action/accept/rate-limit/system-cpu-policer"
bin/schemac schema get --name srl --version $version --vendor Nokia --path "/interface/qos/output/scheduler"
bin/schemac schema get --name srl --version $version --vendor Nokia --path "/interface/qos/output/scheduler/scheduler-policy"
bin/schemac schema get --name srl --version $version --vendor Nokia --path "/interface/qos/output/scheduler/tier"
bin/schemac schema get --name srl --version $version --vendor Nokia --path "/interface/qos/output/scheduler/tier/node"
bin/schemac schema get --name srl --version $version --vendor Nokia --path "/interface/qos/output/scheduler/tier/node[node-number=*]"
bin/schemac schema get --name srl --version $version --vendor Nokia --path "/interface/qos/output/scheduler/tier/node/node-number"

bin/schemac schema to-path --name srl --version $version --vendor Nokia --cp interface,mgmt0,admin-state
bin/schemac schema to-path --name srl --version $version --vendor Nokia --cp acl,cpm-filter,ipv4-filter,entry,1,action,accept,rate-limit,system-cpu-policer
bin/schemac schema expand --name srl --version $version --vendor Nokia --path "interface[name=ethernet-1/1]"
bin/schemac schema expand --name srl --version $version --vendor Nokia --path "/interface/qos"
#
bin/schemac schema bench --name srl --version $version --vendor Nokia --path /
```

### SROS

```shell
bin/schemac schema get --name sros --version 22.10 --vendor Nokia --path /configure/system/name
bin/schemac schema get --name sros --version 22.10 --vendor Nokia --path /configure/service
bin/schemac schema get --name sros --version 22.10 --vendor Nokia --path /configure/filter/ip-filter/entry/match

bin/schemac schema to-path --name sros --version 22.10 --vendor Nokia --cp configure,service,vprn,srv1,interface
bin/schemac schema to-path --name sros --version 22.10 --vendor Nokia --cp configure,filter,ip-filter,filter-1,entry,1,action,nat
bin/schemac schema to-path --name sros --version 22.10 --vendor Nokia --path configure,filter,ip-filter,f1,entry,1,match

bin/schemac schema expand --name sros --version 22.10 --vendor Nokia --path /configure/service --xpath
bin/schemac schema expand --name sros --version 22.10 --vendor Nokia --path /configure/filter/ip-filter --xpath
#
bin/schemac schema bench --name sros --version 22.10 --vendor Nokia --path /configure
```
