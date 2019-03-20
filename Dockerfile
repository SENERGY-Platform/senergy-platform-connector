FROM golang:1.11

COPY . /go/src/senergy-platform-connector
WORKDIR /go/src/senergy-platform-connector

ENV GO111MODULE=on

RUN go build

EXPOSE 8080

ENTRYPOINT ["/go/src/senergy-platform-connector/senergy-platform-connector"]