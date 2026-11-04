FROM golang:1.22-alpine AS builder

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY main.go ./
RUN CGO_ENABLED=0 GOOS=linux go build -o agenda-tec .

FROM alpine:3.19

WORKDIR /app
COPY --from=builder /app/agenda-tec ./agenda-tec
COPY public ./public
ENV HOST=0.0.0.0
ENV PORT=8080
EXPOSE 8080

CMD ["./agenda-tec"]
