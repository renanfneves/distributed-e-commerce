FROM golang:1.26

WORKDIR /go/src/
COPY . .
RUN GOOS=linux go build -ldflags="-s -w" -o catalog .
CMD ["./catalog"]
