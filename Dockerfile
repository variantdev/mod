FROM golang:1.12 as builder

ARG MOD_VERSION

ENV GOOS=linux
ENV GOARCH=amd64

WORKDIR /go/src/github.com/variantdev/mod
COPY . /go/src/github.com/variantdev/mod

RUN if [ -n "${MOD_VERSION}" ]; then git checkout -b tag refs/tags/v${MOD_VERSION}; fi \
    && make build -e GO111MODULE=on


FROM buildpack-deps:scm

COPY --from=builder /go/src/github.com/variantdev/mod/mod /usr/local/bin/mod

ENTRYPOINT ["/usr/local/bin/mod"]
CMD ["--help"]