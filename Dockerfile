FROM golang:1.13 as builder

LABEL maintainer="Han-Wen Nienhuys <hanwen@google.com>"

# Set the Current Working Directory inside the container
WORKDIR /app

# Copy go mod and sum files
COPY go.mod go.sum ./

RUN go version

# Download all dependencies. Dependencies will be cached if the go.mod and go.sum files are not changed
RUN go mod download

# Copy the source from the current directory to the Working Directory inside the container
COPY . .

# Build the Go app. The netgo tag ensures we build a static binary.
RUN go build -tags netgo -o gerrit-linter ./cmd/checker
RUN go build -tags netgo -o buildifier github.com/bazelbuild/buildtools/buildifier
RUN curl -L -o google-java-format.jar https://github.com/google/google-java-format/releases/download/google-java-format-1.7/google-java-format-1.7-all-deps.jar
RUN chmod +x google-java-format.jar
RUN cp $(which gofmt) .

FROM alpine:latest

RUN apk --no-cache add ca-certificates

WORKDIR /app/

# Copy the Pre-built binary file from the previous stage
COPY --from=builder /app/gerrit-linter .
COPY --from=builder /app/gofmt .
COPY --from=builder /app/buildifier .
COPY --from=builder /app/google-java-format.jar .
ENTRYPOINT [ "/app/gerrit-linter" ]
CMD []
