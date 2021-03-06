FROM golang:1.9.3-alpine3.7
MAINTAINER help me docker

run apk update && apk upgrade && apk add --no-cache bash git openssh

RUN mkdir -p /go/src/github.com/moonshot-trading/quote-server
ADD . /go/src/github.com/moonshot-trading/quote-server
RUN go get github.com/moonshot-trading/quote-server
RUN go install github.com/moonshot-trading/quote-server

ENTRYPOINT /go/bin/quote-server
EXPOSE 44417