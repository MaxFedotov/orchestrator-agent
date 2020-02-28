#!/bin/sh
getent group mysql >/dev/null || groupadd -r mysql
getent passwd mysql >/dev/null || \
    useradd -r -g mysql -s /bin/bash \
    -c "MySQL server" mysql
exit 0