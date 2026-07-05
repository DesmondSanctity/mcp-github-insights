# syntax=docker/dockerfile:1

# Static-PIE build: a fully static position-independent executable with no dynamic
# interpreter, so it runs on FROM scratch (no libc/loader needed). Unikraft's ELF
# loader requires PIE; the metro is x86_64, so build for amd64 (external linking
# via CGO gives us -static-pie; netgo avoids libc for DNS).
FROM --platform=linux/amd64 golang:1.25-bookworm AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=1 GOOS=linux GOARCH=amd64 go build \
      -buildmode=pie \
      -ldflags "-linkmode external -extldflags -static-pie -s -w" \
      -tags netgo \
      -o /mcp-server .

FROM scratch
COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=build /mcp-server /mcp-server
ENTRYPOINT ["/mcp-server"]
