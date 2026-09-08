FROM golang:1.23-alpine AS build
WORKDIR /src
COPY . .
RUN go build -o /api-server .
FROM alpine:3.20
COPY --from=build /api-server /api-server
EXPOSE 8080
ENTRYPOINT ["/api-server"]
