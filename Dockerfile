FROM --platform=$BUILDPLATFORM node:24-alpine AS web
WORKDIR /src/web
COPY web/package*.json ./
RUN npm ci
COPY web/ ./
RUN npm run build

FROM --platform=$BUILDPLATFORM golang:1.25-alpine AS server
WORKDIR /src
ARG TARGETOS
ARG TARGETARCH
RUN apk add --no-cache ca-certificates git
COPY go.mod go.sum* ./
RUN go mod download
COPY . .
COPY --from=web /src/web/dist ./web/dist
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath -ldflags="-s -w" -o /out/tvpn ./cmd/tvpn

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=server /out/tvpn /tvpn
EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["/tvpn"]
