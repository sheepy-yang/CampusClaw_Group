# 骨架 Dockerfile：实现阶段再补 multi-stage build
FROM golang:1.22-alpine AS build
WORKDIR /src
COPY . .
RUN go build -o /out/server ./cmd/server && go build -o /out/worker ./cmd/worker

FROM alpine:3.20
WORKDIR /app
COPY --from=build /out/server /out/worker /app/
EXPOSE 8080
CMD ["/app/server"]
