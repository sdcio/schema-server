#!/bin/bash

num_vlans=10

SCRIPTPATH="$( cd -- "$(dirname "$0")" >/dev/null 2>&1 ; pwd -P )"
CLIENT=$SCRIPTPATH/../../bin/client
BULK=$SCRIPTPATH/$BULK


#$CLIENT schema get --name srl --version 22.11.1 --vendor Nokia --path /interface[name=ethernet-1/1]/subinterface

echo "srl1/ethernet-1/1"
$BULK -a clab-distributed-data-server:56000 --ds srl1 --candidate default1 --vlans $num_vlans --interface ethernet-1/1 #--cleanup #&

echo "srl2/ethernet-1/1"
$BULK -a clab-distributed-data-server:56000 --ds srl2 --candidate default1 --vlans $num_vlans --interface ethernet-1/1 #--cleanup #&

echo "srl3/ethernet-1/1"
$BULK -a clab-distributed-data-server:56000 --ds srl3 --candidate default1 --vlans $num_vlans --interface ethernet-1/1 #--cleanup #&

echo "srl3/ethernet-1/2"
$BULK -a clab-distributed-data-server:56000 --ds srl3 --candidate default2 --vlans $num_vlans --interface ethernet-1/2 #--cleanup