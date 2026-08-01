FROM node:24-alpine AS web
WORKDIR /src/web
COPY web/package*.json ./
RUN npm ci
COPY web/ ./
RUN npm run build

FROM golang:1.25-alpine AS server
WORKDIR /src
RUN apk add --no-cache ca-certificates git
COPY go.mod go.sum* ./
RUN go mod download
COPY . .
COPY --from=web /src/web/dist ./web/dist
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/tvpn ./cmd/tvpn

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=server /out/tvpn /tvpn
EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["/tvpn"]

