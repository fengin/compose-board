set GOOS=linux
set GOARCH=amd64
go build -ldflags "-s -w -X main.Version=1.1.0" -o composeboard_linux_amd64 main.go
