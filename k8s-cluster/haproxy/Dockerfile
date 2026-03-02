FROM haproxy:2.9-alpine

COPY haproxy.cfg /usr/local/etc/haproxy/haproxy.cfg

EXPOSE 80 8080 9090

CMD ["haproxy", "-f", "/usr/local/etc/haproxy/haproxy.cfg"]
