.PHONY: build
build:
	go build

.PHONY: test
test:
	go test ./... -args -test.v

.PHONY: fmt
fmt:
	go fmt $(go list ./... | grep -v /vendor/)

.PHONY: goreleaser
goreleaser:
	docker run --rm -v "$(PWD)":/go/src/github.com/variantdev/mod -w /go/src/github.com/variantdev/mod goreleaser/goreleaser:latest release --snapshot --clean

release/minor:
	git checkout master
	git pull --rebase origin master
	bash -c 'if git branch | grep autorelease; then git branch -D autorelease; else echo no branch to be cleaned; fi'
	git checkout -b autorelease origin/master
	bash -c 'SEMTAG_REMOTE=origin hack/semtag final -s minor'
	git checkout master

release/patch:
	git checkout master
	git pull --rebase origin master
	bash -c 'if git branch | grep autorelease; then git branch -D autorelease; else echo no branch to be cleaned; fi'
	git checkout -b autorelease origin/master
	bash -c 'SEMTAG_REMOTE=origin hack/semtag final -s patch'
	git checkout master
