FROM golang:1.18-alpine as builder

ARG BUILD_INFO

ENV GOPATH=/root/go CGO_ENABLED=0

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
    go install \
        -ldflags "-w -X 'github.com/Pix4D/cogito/resource.buildinfo=$BUILD_INFO'" \
        ./cmd/cogito

#
# The final image
#

FROM alpine

RUN apk --no-cache add ca-certificates

RUN mkdir -p /opt/resource

COPY --from=builder /root/go/bin/* /opt/resource/

RUN ln -s /opt/resource/cogito /opt/resource/check && \
    ln -s /opt/resource/cogito /opt/resource/in && \
    ln -s /opt/resource/cogito /opt/resource/out
