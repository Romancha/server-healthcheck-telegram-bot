FROM golang:alpine as builder
LABEL maintainer="romanchabest55@gmail.com"

RUN apk update && apk add --no-cache git ca-certificates tzdata && update-ca-certificates

WORKDIR $GOPATH/src/mypackage/myapp/
COPY . .

RUN go get -d -v
RUN go mod download

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags='-w -s -extldflags "-static"' -a \
    -o /go/bin/app .

FROM alpine:3

RUN apk add --no-cache ca-certificates tzdata curl

COPY --from=builder /go/bin/app /go/bin/app

EXPOSE 8081

HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
    CMD curl -f http://localhost:8081/health || exit 1

ENTRYPOINT ["/go/bin/app"]
