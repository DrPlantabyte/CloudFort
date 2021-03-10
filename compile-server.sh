#!/bin/bash
cd "$(dirname "$0")/src"
go build -o ../build/ cmd/CloudFort-Server.go
cd ..

