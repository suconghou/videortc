build:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -v -o videortc -a -ldflags "-s -w" main.go

