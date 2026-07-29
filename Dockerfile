# Maintenance Hub API. Build context = repo root (the server applies migrations
# from ../migrations at boot, so both api/ and migrations/ must be in the image).
FROM golang:1.24-alpine AS build
WORKDIR /src
COPY api/go.mod api/go.sum ./api/
RUN cd api && go mod download
COPY api ./api
RUN cd api && CGO_ENABLED=0 GOOS=linux go build -o /out/server ./cmd/server \
 && CGO_ENABLED=0 GOOS=linux go build -o /out/provision ./cmd/provision

FROM alpine:3.20
RUN apk add --no-cache ca-certificates
WORKDIR /app
COPY --from=build /out/server /app/server
COPY --from=build /out/provision /app/provision
COPY migrations /app/migrations
ENV MIGRATIONS_DIR=/app/migrations
# The host injects PORT; the server falls back to 8091 locally.
EXPOSE 8091
CMD ["/app/server"]
