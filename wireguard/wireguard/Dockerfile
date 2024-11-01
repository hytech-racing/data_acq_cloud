FROM alpine:latest

# Install WireGuard tools and other utilities
RUN apk add --no-cache wireguard-tools openrc bash coreutils gettext

# Create directories for WireGuard keys and conf
RUN mkdir -p /etc/wireguard/keys
COPY wg0.conf.template /etc/wireguard/wg0.conf.template

RUN chmod 700 /etc/wireguard/keys
RUN echo "net.ipv4.ip_forward=1" >> /etc/sysctl.conf
CMD sh -c "envsubst < /etc/wireguard/wg0.conf.template > /etc/wireguard/wg0.conf && wg-quick up wg0 && tail -f /dev/null"
