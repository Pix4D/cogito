FROM golang:1.16-alpine as builder

ARG BUILD_INFO

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

# RUN go test ./...  && \ 
RUN go install \
        -ldflags "-w -X 'github.com/Pix4D/cogito/resource.buildinfo=$BUILD_INFO'" \
        ./cmd/check \
        ./cmd/in \
        ./cmd/out

#
# The final image
#

FROM alpine

RUN apk --no-cache add ca-certificates

RUN mkdir -p /opt/resource

COPY --from=builder /root/go/bin/* /opt/resource/
