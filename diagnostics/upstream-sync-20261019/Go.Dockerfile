FROM golang:1.26.9-alpine
ENV GOMAXPROCS=2 GOFLAGS=-p=1 GOMEMLIMIT=3GiB
RUN apk add --no-cache build-base git ca-certificates tzdata
