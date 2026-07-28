FROM node:22-alpine AS web-build
WORKDIR /src/web
COPY web/package.json ./
RUN npm install --ignore-scripts
COPY web/ ./
RUN npm run build

FROM golang:1.26-alpine AS go-build
WORKDIR /src
RUN apk add --no-cache git ca-certificates
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=web-build /src/web/dist ./web/dist
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/chatops-deploy ./cmd/server

FROM gcr.io/distroless/static-debian12:nonroot
WORKDIR /app
COPY --from=go-build /out/chatops-deploy /app/chatops-deploy
COPY --from=web-build /src/web/dist /app/web/dist
USER nonroot:nonroot
EXPOSE 8080
ENTRYPOINT ["/app/chatops-deploy"]
