#!/bin/bash
cd "$(dirname "$0")/src"
go build -o ../build CloudFort.go CloudFortCore.go Util.go
cd ..