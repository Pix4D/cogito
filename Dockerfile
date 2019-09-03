# Sigh. Something in the dependencies wants the gcc compiler, I get:
#   Step 9/12 : RUN go test -v ./...  &&     go install ./...
#   ---> Running in b4a3217b40a4
#   # runtime/cgo
#   exec: "gcc": executable file not found in $PATH
# FROM golang:1.13-alpine as builder

# FROM golang:1.13 as builder
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
#
COPY go.mod go.sum ./
RUN go mod download

#
# Build
#

COPY . .

RUN go test ./...  && \
    go install -ldflags "-X 'github.com/Pix4D/cogito/resource.version=$VERSION' -X 'github.com/Pix4D/cogito/resource.commit=$COMMIT' -X 'github.com/Pix4D/cogito/resource.date=$DATE'" ./...

#
# The final image
#

# I think that to use `scratch` I need to put a shell there.
# FROM scratch
# See the Sigh above for alpine
# This one is 5MB
FROM alpine 

# This one is 30MB, not that big at the end.
# FROM busybox:glibc

RUN apk --no-cache add ca-certificates

RUN mkdir -p /opt/resource

COPY --from=builder /root/go/bin/* /opt/resource/
