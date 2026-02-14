FROM nginxinc/nginx-unprivileged:alpine

COPY src/index.html /usr/share/nginx/html/index.html

EXPOSE 8080
