# Stage 1: Build the Go Lambda binary
FROM golang:1.26-alpine AS builder

WORKDIR /app

# Install ca-certificates and git for Go module resolution
RUN apk add --no-cache ca-certificates git

# Cache dependencies layer
COPY go.mod go.sum ./
RUN go mod download

# Copy source files
COPY . .

# Build static binary for AWS Lambda provided runtime
ARG TARGETOS=linux
ARG TARGETARCH=arm64
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build \
    -tags lambda.norpc \
    -ldflags="-s -w" \
    -o bootstrap .

# Stage 2: AWS Lambda Runtime Environment
FROM public.ecr.aws/lambda/provided:al2023

# Copy compiled binary from builder stage into Lambda task root
COPY --from=builder /app/bootstrap ${LAMBDA_TASK_ROOT}/bootstrap

# Ensure binary is executable
RUN chmod +x ${LAMBDA_TASK_ROOT}/bootstrap

# Set Lambda handler entrypoint
CMD [ "bootstrap" ]
