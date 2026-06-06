FROM mirror.ccs.tencentyun.com/library/golang:latest AS builder
COPY . /usr/local/go/src/upload-actions
WORKDIR /usr/local/go/src/upload-actions
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 GO111MODULE=on GOPROXY=https://mirrors.tencentyun.com/go,direct go build -o /usr/bin/upload-actions upload-actions

###
FROM mirror.ccs.tencentyun.com/library/ubuntu:latest AS final
COPY --from=builder /usr/bin/upload-actions /usr/bin/
ENTRYPOINT ["/usr/bin/upload-actions"]
