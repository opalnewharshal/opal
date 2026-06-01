# Multi-stage build for the OPAL controller manager.
# Stage 1: Build the Go binary
FROM golang:1.22-alpine AS builder

ARG TARGETOS=linux
ARG TARGETARCH=amd64

WORKDIR /workspace

# Cache go modules separately for faster rebuilds
COPY go.mod go.mod
COPY go.sum go.sum
RUN go mod download

# Copy source
COPY cmd/       cmd/
COPY api/       api/
COPY internal/  internal/

# Build the manager binary
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go build -trimpath -a -o manager ./cmd/main.go

# Stage 2: Minimal runtime image
FROM gcr.io/distroless/static:nonroot

WORKDIR /

COPY --from=builder /workspace/manager .

# Run as nonroot user (distroless nonroot UID = 65532)
USER 65532:65532

ENTRYPOINT ["/manager"]
