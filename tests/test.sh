#!/bin/bash

SCRIPTPATH="$( cd -- "$(dirname "$0")" >/dev/null 2>&1 ; pwd -P )"
CLIENT=$SCRIPTPATH/../bin/schemac
HOST=localhost:55000
version=23.3.2

$CLIENT -a $HOST schema list

$CLIENT -a $HOST schema get --name srl --version $version --vendor Nokia --path /interface[name=ethernet-1/1]/subinterface
$CLIENT -a $HOST schema get --name srl --version $version --vendor Nokia --path /interface[name=ethernet-1/1]/subinterface --all
$CLIENT -a $HOST schema get --name srl --version $version --vendor Nokia --path /acl/cpm-filter/ipv4-filter/entry/action/accept/rate-limit/system-cpu-policer
$CLIENT -a $HOST schema to-path --name srl --version $version --vendor Nokia --cp interface,mgmt0,admin-state
$CLIENT -a $HOST schema to-path --name srl --version $version --vendor Nokia --cp acl,cpm-filter,ipv4-filter,entry,1,action,accept,rate-limit,system-cpu-policer
$CLIENT -a $HOST schema expand --name srl --version $version --vendor Nokia --path interface[name=ethernet-1/1]

$CLIENT -a $HOST schema bench --name srl --vendor Nokia --version $version

echo "start"
date -Ins
for i in $(seq 1 1000);
do 
    #date -Ins
    $CLIENT -a $HOST schema get --name srl --version $version --vendor Nokia --path /interface[name=ethernet-1/1]/subinterface > /dev/null
    # $CLIENT -a $HOST schema to-path --name srl --version $version --vendor Nokia --cp acl,cpm-filter,ipv4-filter,entry,1,action,accept,rate-limit,system-cpu-policer > /dev/null
    # $CLIENT -a $HOST schema expand --name srl --version $version --vendor Nokia --path interface[name=ethernet-1/1] > /dev/null
    #date -Ins
done
date -Ins
