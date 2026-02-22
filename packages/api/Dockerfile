FROM golang:1.24-alpine AS build

WORKDIR /src

COPY go.mod ./

RUN go mod download

COPY . .

RUN go mod tidy

RUN go build -o /out/agentqa-api ./cmd/api

FROM alpine:3.20

RUN apk add --no-cache ca-certificates

WORKDIR /app

COPY --from=build /out/agentqa-api /app/agentqa-api

EXPOSE 8080

CMD ["/app/agentqa-api"]
