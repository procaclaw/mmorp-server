FROM golang:1.22-alpine AS builder
WORKDIR /src
COPY go.mod ./
COPY . .
RUN go mod tidy && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/mmorp-server ./cmd/server

FROM gcr.io/distroless/static-debian12
WORKDIR /app
COPY --from=builder /out/mmorp-server /app/mmorp-server
COPY migrations /app/migrations
COPY data /app/data
EXPOSE 8080
ENTRYPOINT ["/app/mmorp-server"]
