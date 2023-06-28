# SCHEMA-SERVER

## build

```shell
make build
```

## run the server

```shell
./bin/server
```

## run the client

### srl

```shell
# schema
bin/schemac schema get --name srl --version 22.11.1 --vendor Nokia --path /interface[name=ethernet-1/1]/subinterface
bin/schemac schema get --name srl --version 22.11.1 --vendor Nokia --path /acl/cpm-filter/ipv4-filter/entry/action/accept/rate-limit/system-cpu-policer
bin/schemac schema to-path --name srl --version 22.11.1 --vendor Nokia --cp interface,mgmt0,admin-state
bin/schemac schema expand --name srl --version 22.11.1 --vendor Nokia --path interface[name=ethernet-1/1]
#
bin/schemac schema bench --name srl --version 22.11.1 --vendor Nokia --path /
```

### SROS

```shell
bin/schemac schema get --name sros --version 22.10 --vendor Nokia --path /configure/system/name
bin/schemac schema get --name sros --version 22.10 --vendor Nokia --path /configure/service
bin/schemac schema to-path --name sros --version 22.10 --vendor Nokia --cp configure,service,vprn,v1,interface
bin/schemac schema expand --name sros --version 22.10 --vendor Nokia --path /configure/service --xpath
#
bin/schemac schema bench --name srl --version 22.10 --vendor Nokia --path /configure
```
