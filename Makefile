.PHONY: build
build:
	go build

.PHONY: test
test:
	go test ./... -args -test.v

.PHONY: fmt
fmt:
	go fmt $(go list ./... | grep -v /vendor/)
