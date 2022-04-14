dev:
	ID=SSe0eddb-40ed-da57-f581-3e996e5345SS WS_ADDR="ws://127.0.0.1:6060/" go run -race main.go -p 6080

build:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -v -o videortc -a -ldflags "-s -w" main.go

docker:
	make build && \
	docker build -t=suconghou/videortc .