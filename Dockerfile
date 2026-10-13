# syntax=docker/dockerfile:1

# Web widget, built for the http API: it calls /v1 on the origin that serves
# it. Dependencies are installed before the sources are copied so that the
# layer survives source changes.
FROM node:22-alpine AS web
WORKDIR /src/web
COPY web/package.json web/package-lock.json ./
RUN npm ci
COPY web ./
RUN npm run build

FROM golang:1.24-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd ./cmd
COPY internal ./internal
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/slotbook ./cmd/slotbook

FROM gcr.io/distroless/static-debian12:nonroot
WORKDIR /app
COPY --from=build /out/slotbook /app/slotbook
COPY --from=web /src/web/dist /app/web
COPY seed /app/seed
ENV SLOTBOOK_ADDR=:8080 \
    SLOTBOOK_STATIC_DIR=/app/web
EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["/app/slotbook"]
CMD ["serve"]
