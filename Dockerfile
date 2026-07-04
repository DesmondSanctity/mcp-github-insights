# syntax=docker/dockerfile:1

FROM golang:1.25-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
# Unikraft's ELF loader requires a position-independent executable (PIE), built
# for the x86_64 (amd64) metro. The resulting dynamic PIE needs the musl loader,
# which the alpine final stage below provides (a scratch image cannot).
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -buildmode=pie -ldflags "-s -w" -o /mcp-server .

FROM --platform=linux/amd64 alpine:3.20
RUN apk add --no-cache ca-certificates
COPY --from=build /mcp-server /mcp-server
ENTRYPOINT ["/mcp-server"]
