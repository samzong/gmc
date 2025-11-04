FROM golang:1.24-alpine AS builder

WORKDIR /build

# Install git for version detection
RUN apk add --no-cache git

# Copy go mod files
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build the binary
ARG VERSION=dev
ARG BUILDTIME=unknown
RUN CGO_ENABLED=0 go build -trimpath \
    -ldflags "-s -w -X github.com/samzong/gmc/cmd.Version=${VERSION} -X github.com/samzong/gmc/cmd.BuildTime=${BUILDTIME}" \
    -o gmc .

FROM alpine:latest

RUN apk --no-cache add ca-certificates git

WORKDIR /root

# Copy the binary from builder
COPY --from=builder /build/gmc /usr/local/bin/gmc

# Make sure gmc is executable
RUN chmod +x /usr/local/bin/gmc

# Set the entrypoint
ENTRYPOINT ["gmc"]
CMD ["--help"]

