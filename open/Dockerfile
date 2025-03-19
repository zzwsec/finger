FROM  alpinelinux/ansible:latest
WORKDIR /open
COPY . .
RUN sed -i 's#https\?://dl-cdn.alpinelinux.org/alpine#https://mirrors.tuna.tsinghua.edu.cn/alpine#g' /etc/apk/repositories
RUN apk update \
    && apk add --no-cache tzdata \
    && cp /usr/share/zoneinfo/Asia/Shanghai /etc/localtime \
    && echo Asia/Shanghai > /etc/timezone \
    && apk del tzdata \
    && chmod +x open-linux \
    && mkdir -p /var/log/ansible

ENTRYPOINT ["/open/open-linux"]