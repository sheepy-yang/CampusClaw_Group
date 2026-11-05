FROM golang:1.22-alpine AS build
WORKDIR /src
RUN apk add --no-cache ca-certificates
COPY go.mod ./
COPY . .
RUN CGO_ENABLED=0 go build -o /out/server ./cmd/server && CGO_ENABLED=0 go build -o /out/worker ./cmd/worker

FROM alpine:3.20
RUN apk add --no-cache ca-certificates wget
WORKDIR /app
COPY --from=build /out/server /out/worker /app/
EXPOSE 8080
CMD ["/app/server"]
