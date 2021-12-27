VERSION 0.6

FROM golang:1.17-alpine3.14

RUN apk add --update --no-cache \
    bash \
    bash-completion \
    binutils \
    ca-certificates \
    coreutils \
    curl \
    findutils \
    g++ \
    git \
    grep \
    less \
    make \
    openssl \
    util-linux

WORKDIR /acbup

deps:
    FROM +base
    RUN go get golang.org/x/tools/cmd/goimports
    RUN go get golang.org/x/lint/golint
    RUN go get github.com/gordonklaus/ineffassign
    COPY go.mod go.sum ./
    RUN go mod download
    SAVE ARTIFACT go.mod AS LOCAL go.mod
    SAVE ARTIFACT go.sum AS LOCAL go.sum

code:
    FROM +deps
    COPY --dir pack util cmd ./

lint:
    FROM +code
    RUN output="$(ineffassign ./... 2>&1)" ; \
        if [ -n "$output" ]; then \
            echo "$output" ; \
            exit 1 ; \
        fi
    RUN output="$(goimports -d $(find . -type f -name '*.go') 2>&1)"  ; \
        if [ -n "$output" ]; then \
            echo "$output" ; \
            exit 1 ; \
        fi
    RUN golint -set_exit_status ./...
    RUN output="$(go vet ./... 2>&1)" ; \
        if [ -n "$output" ]; then \
            echo "$output" ; \
            exit 1 ; \
        fi
unit-test:
    FROM +code
    RUN go test ./...

test:
    BUILD +lint
    BUILD +unit-test
    BUILD ./tests+all

acbup:
    FROM +code
    RUN go build -o build/acbup cmd/acbup/*.go
    SAVE ARTIFACT build/acbup AS LOCAL "build/acbup"
