mkdir -p "$PWD/searxng/config" "$PWD/searxng/data"
docker run -d --name searxng \
  -p 127.0.0.1:13004:8080 \
  -v "$PWD/searxng/config:/etc/searxng" \
  -v "$PWD/searxng/data:/var/cache/searxng" \
  docker.io/searxng/searxng:latest