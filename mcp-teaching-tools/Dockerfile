FROM golang:1.22-alpine AS build
WORKDIR /src
COPY go.mod ./
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/mcp-server ./cmd/server

FROM alpine:3.20
RUN apk add --no-cache ca-certificates \
    && addgroup -S campusclaw \
    && adduser -S -G campusclaw campusclaw
WORKDIR /app
COPY --from=build /out/mcp-server /app/mcp-server
USER campusclaw
EXPOSE 8100
CMD ["/app/mcp-server"]
