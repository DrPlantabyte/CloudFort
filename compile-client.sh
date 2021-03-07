#!/bin/bash
cd "$(dirname "$0")"
go build -o ../build CloudFort.go CloudFortCore.go Util.go
cd ..