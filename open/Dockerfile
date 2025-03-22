FROM alpine:3.21.3

WORKDIR /open

COPY . .

RUN sed -i 's#https\?://dl-cdn.alpinelinux.org/alpine#https://mirrors.tuna.tsinghua.edu.cn/alpine#g' /etc/apk/repositories \
    && apk update \
    && apk add --no-cache tzdata ansible openssh-client \
    && cp /usr/share/zoneinfo/Asia/Shanghai /etc/localtime \
    && echo Asia/Shanghai > /etc/timezone \
    && apk del tzdata \
    && mkdir -p /var/log/ansible \
    && chmod +x open-linux

ENTRYPOINT ["/open/open-linux"]