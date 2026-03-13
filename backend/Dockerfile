FROM golang:1.23-alpine AS build
WORKDIR /app
COPY go.mod go.sum ./
COPY vendor/ vendor/
COPY . .
RUN CGO_ENABLED=0 go build -mod=vendor -o server .

FROM alpine:3.19
RUN apk add --no-cache git ca-certificates
WORKDIR /app
COPY --from=build /app/server .
EXPOSE 8080
CMD ["./server"]
