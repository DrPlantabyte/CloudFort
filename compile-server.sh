#!/bin/bash
cd "$(dirname "$0")/src"
go build -o ../build CloudFort-Server.go CloudFortCore.go Util.go DemoWorld.go
cd ..

