FROM node:22-alpine AS frontend
WORKDIR /src/frontend
COPY frontend/package.json frontend/package-lock.json ./
RUN npm ci
COPY frontend/ ./
RUN npm run build

FROM golang:1.26.5-alpine AS backend
WORKDIR /src/backend
COPY backend/go.mod backend/go.sum ./
RUN go mod download
COPY backend/ ./
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/viki ./cmd/viki

FROM alpine:3.22
RUN apk add --no-cache ca-certificates wget && addgroup -S viki && adduser -S -G viki viki
WORKDIR /app
COPY --from=backend /out/viki /app/viki
COPY --from=frontend /src/frontend/dist /app/public
USER viki
EXPOSE 8080
ENTRYPOINT ["/app/viki"]
