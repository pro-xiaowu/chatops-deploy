FROM node:22-alpine AS web-build
WORKDIR /src/web
COPY web/package.json ./
COPY web/package-lock.json ./
RUN npm ci --ignore-scripts
COPY web/ ./
RUN npm run build

FROM golang:1.25-alpine AS go-build
WORKDIR /src
RUN apk add --no-cache git ca-certificates
ENV GOPROXY=https://goproxy.cn,direct
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=web-build /src/web/dist ./web/dist
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/chatops-deploy ./cmd/server

FROM scratch
WORKDIR /app
COPY --from=go-build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=go-build /out/chatops-deploy /app/chatops-deploy
COPY --from=web-build /src/web/dist /app/web/dist
USER 65532:65532
EXPOSE 8080
ENTRYPOINT ["/app/chatops-deploy"]
