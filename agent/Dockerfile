FROM golang:1.23-alpine AS build
WORKDIR /src
COPY . .
RUN go build -o /agent-server .
FROM alpine:3.20
COPY --from=build /agent-server /agent-server
EXPOSE 8090
ENTRYPOINT ["/agent-server"]
