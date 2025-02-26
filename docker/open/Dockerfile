FROM  alpinelinux/ansible:latest

WORKDIR /open

COPY . .

RUN set -ex \
    && rm -rf Dockerfile args.yaml init.txt game_list.txt \
    && sed -i 's#https\?://dl-cdn.alpinelinux.org/alpine#https://mirrors.tuna.tsinghua.edu.cn/alpine#g' /etc/apk/repositories \
    && apk add --no-cache tzdata \
    && cp /usr/share/zoneinfo/Asia/Shanghai /etc/localtime \
    && echo Asia/Shanghai > /etc/timezone \
    && apk del tzdata \
    && chmod +x open-linux \
    && mkdir -p /var/log/ansible

CMD ["/open/open-linux"]