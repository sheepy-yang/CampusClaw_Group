FROM golang:1.22-alpine AS build
WORKDIR /src
COPY . .
RUN go build -o /out/mcp-server ./cmd/server

FROM alpine:3.20
WORKDIR /app
COPY --from=build /out/mcp-server /app/
EXPOSE 8100
CMD ["/app/mcp-server"]
