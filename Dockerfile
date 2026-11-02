FROM golang:1.27.3-bookworm AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o /out/api ./cmd/api && \
    CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o /out/migrate ./cmd/migrate && \
    CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o /out/import ./cmd/import && \
    CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o /out/healthcheck ./cmd/healthcheck

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/ /
USER nonroot:nonroot
EXPOSE 8080
ENTRYPOINT ["/api"]
