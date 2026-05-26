FROM golang:1.25-alpine AS build

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /app/bin/app ./internal

FROM alpine:3.20

WORKDIR /app

COPY --from=build /app/bin/app /app/app

EXPOSE 8080
ENV SERVER_ADDR=:8080

CMD ["/app/app"]
