FROM node:22.11-alpine AS web
WORKDIR /src
COPY web/package.json web/package-lock.json ./
RUN npm ci
COPY web .
RUN npm run build

FROM golang:1.23.2-alpine AS api
WORKDIR /src
COPY server/go.mod server/go.sum* ./
RUN go mod download
COPY server .
RUN CGO_ENABLED=0 go build -o /pac ./cmd/pac

FROM alpine:3.20
RUN addgroup -S pac && adduser -S pac -G pac && mkdir -p /data/uploads && chown -R pac:pac /data
COPY --from=api /pac /app/pac
COPY --from=web /src/dist /app/web
USER pac
EXPOSE 8080
ENTRYPOINT ["/app/pac"]

