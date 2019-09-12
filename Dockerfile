FROM golang:1.13-alpine as builder

ARG VERSION
ARG COMMIT
ARG DATE

ENV GO111MODULE=on
ENV GOPATH=/root/go

RUN apk --no-cache add build-base

WORKDIR /code

#
# Optimize downloading of dependencies only when they are needed.
# This requires to _first_ copy only these two files, run `go mod download`,
# and _then_ copy the rest of the source code.
#
COPY go.mod go.sum ./
RUN go mod download

#
# Build.
#
COPY . .

RUN go test ./...  && \
    go install -ldflags "-X 'github.com/Pix4D/cogito/resource.version=$VERSION' -X 'github.com/Pix4D/cogito/resource.commit=$COMMIT' -X 'github.com/Pix4D/cogito/resource.date=$DATE'" ./...

#
# The final image
#

FROM alpine

# This one is 30MB, not that big at the end.
# FROM busybox:glibc

RUN apk --no-cache add ca-certificates

RUN mkdir -p /opt/resource

COPY --from=builder /root/go/bin/* /opt/resource/
