# syntax=docker/dockerfile:1.7
FROM golang:1.24.7-alpine3.22 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -o /out/server ./cmd/server \
    && CGO_ENABLED=0 go build -trimpath -o /out/migrate ./cmd/migrate

FROM alpine:3.22.1
RUN addgroup -S layr && adduser -S -G layr layr
COPY --from=build /out/server /usr/local/bin/layr-server
COPY --from=build /out/migrate /usr/local/bin/layr-migrate
USER layr
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/layr-server"]
