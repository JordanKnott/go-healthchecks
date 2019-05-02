FROM golang:1.11-alpine

MAINTAINER Jordan Knott <jordan@jordanthedev.com>

RUN apk update && apk add git && rm -rf /var/cache/apk/*

COPY . /go/src/github.com/jordanknott/go-healthcheck
WORKDIR /go/src/github.com/jordanknott/go-healthcheck

CMD go build && ./go-healthcheck
