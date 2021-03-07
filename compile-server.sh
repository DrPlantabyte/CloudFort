#!/bin/bash
cd "$(dirname "$0")"
go build -o ../build CloudFort-Server.go CloudFortCore.go Util.go DemoWorld.go
cd ..