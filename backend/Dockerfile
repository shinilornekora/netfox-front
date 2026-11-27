# Один Dockerfile на все сервисы бэкенда: go.work собирает нужный модуль вместе
# с contracts и kit. SERVICE выбирает, что собирать (identity, catalog, billing,
# playback, gateway). Контекст сборки — папка backend/.
FROM golang:1.26-alpine AS build
ARG SERVICE
WORKDIR /src
COPY go.work go.work.sum ./
COPY contracts/go.mod contracts/go.sum* contracts/
COPY kit/go.mod kit/go.sum kit/
COPY tools/go.mod tools/go.sum tools/
COPY services/identity/go.mod services/identity/go.sum services/identity/
COPY services/catalog/go.mod services/catalog/go.sum services/catalog/
COPY services/billing/go.mod services/billing/go.sum services/billing/
COPY services/playback/go.mod services/playback/go.sum services/playback/
COPY services/gateway/go.mod services/gateway/go.sum services/gateway/
RUN --mount=type=cache,target=/root/go/pkg/mod go mod download all
COPY . .
RUN --mount=type=cache,target=/root/go/pkg/mod --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/app ./services/${SERVICE}/cmd

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/app /app
USER nonroot
ENTRYPOINT ["/app"]
