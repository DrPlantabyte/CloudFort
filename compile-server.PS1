cd $PSScriptRoot\src
go build -o ..\build\ CloudFort-Server.go CloudFortCore.go Util.go DemoWorld.go
cd ..