FROM mirror.ccs.tencentyun.com/library/golang:1.26-alpine as builder
COPY . /usr/local/go/src/upload-actions
WORKDIR /usr/local/go/src/upload-actions
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 GO111MODULE=on GOPROXY=https://goproxy.cn go build -o /usr/bin/upload-actions upload-actions

###
FROM mirror.ccs.tencentyun.com/library/alpine as final
RUN sed -i 's/dl-cdn.alpinelinux.org/mirrors.tencentyun.com/g' /etc/apk/repositories
RUN apk --no-cache add ca-certificates
ENTRYPOINT ["/usr/bin/upload-actions"]
COPY --from=builder /usr/bin/upload-actions /usr/bin/