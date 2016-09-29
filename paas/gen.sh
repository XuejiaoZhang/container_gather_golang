#!/bin/bash
#protoc -I helloworld/ helloworld/helloworld.proto --go_out=plugins=grpc:helloworld
protoc -I monitor/ monitor/monitor.proto --go_out=plugins=grpc:monitor
