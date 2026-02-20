FROM golang:1.24-alpine AS build

WORKDIR /app
COPY go.mod ./
COPY main.go ./
COPY src/ ./src/

RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /server .

FROM scratch

COPY --from=build /server /server

EXPOSE 8080

ENTRYPOINT ["/server"]
