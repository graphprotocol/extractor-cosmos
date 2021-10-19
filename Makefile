.PHONU: setup test

setup:
	go mod download

test:
	go test -race -cover ./...
