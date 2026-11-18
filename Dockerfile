FROM golang:1.25 AS build

WORKDIR /src
COPY go.mod go.sum ./
COPY . .
ENV CGO_ENABLED=0
RUN go build -o /out/attendance ./cmd/attendance

FROM alpine:3.22
COPY --from=build /out/attendance /attendance
EXPOSE 8080
ENTRYPOINT ["/attendance"]
