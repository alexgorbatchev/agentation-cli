set positional-arguments

build:
	mkdir -p ../bin
	go build -o ../bin/agentation ./cmd/agentation

test:
	go test ./...

fmt:
	go fmt ./...

lint:
	go test ./...
