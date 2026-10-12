FROM docker.io/library/golang:1.27.2-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/api ./cmd/api \
    && CGO_ENABLED=0 GOBIN=/out go install -tags='no_clickhouse,no_libsql,no_mssql,no_mysql,no_sqlite3,no_vertica,no_ydb' github.com/pressly/goose/v3/cmd/goose@v3.29.0 \
    && go clean -cache -modcache

FROM docker.io/library/alpine:3.23
RUN apk add --no-cache ca-certificates && addgroup -S app && adduser -S -G app app
WORKDIR /app
COPY --from=build /out/api /out/goose /usr/local/bin/
COPY migrations ./migrations
USER app
EXPOSE 8080
CMD ["api"]
