FROM openresty/openresty:1.27.1.2-3-bookworm

ARG MODSECURITY_VERSION=3.0.9
ARG CRS_VERSION=4.14.0
ARG DEBIAN_MIRROR=deb.debian.org
ARG NGINX_VERSION=1.27.1
ARG MODSECURITY_NGINX_VERSION=1.0.3

RUN if [ "$DEBIAN_MIRROR" != "deb.debian.org" ]; then \
      sed -i "s|deb.debian.org|$DEBIAN_MIRROR|g" /etc/apt/sources.list /etc/apt/sources.list.d/* 2>/dev/null || true; \
    fi \
    && apt-get update \
    && apt-get install -y --no-install-recommends build-essential libpcre3-dev zlib1g-dev libssl-dev libmodsecurity3 libmodsecurity-dev wget ca-certificates \
    && rm -rf /var/lib/apt/lists/*

RUN mkdir -p /tmp \
    && wget -qO /tmp/nginx.tar.gz https://nginx.org/download/nginx-${NGINX_VERSION}.tar.gz \
    && tar -xzf /tmp/nginx.tar.gz -C /tmp \
    && wget -qO /tmp/modsecurity-nginx.tar.gz https://github.com/owasp-modsecurity/ModSecurity-nginx/archive/refs/tags/v${MODSECURITY_NGINX_VERSION}.tar.gz \
    && tar -xzf /tmp/modsecurity-nginx.tar.gz -C /tmp \
    && cd /tmp/nginx-${NGINX_VERSION} \
    && ./configure --with-compat --with-ld-opt=-lpcre --add-dynamic-module=/tmp/ModSecurity-nginx-${MODSECURITY_NGINX_VERSION} \
    && make modules \
    && install -m 0644 objs/ngx_http_modsecurity_module.so /usr/lib/ngx_http_modsecurity_module.so \
    && rm -rf /tmp/nginx.tar.gz /tmp/modsecurity-nginx.tar.gz /tmp/nginx-${NGINX_VERSION} /tmp/ModSecurity-nginx-${MODSECURITY_NGINX_VERSION}

COPY runtime/lua/ /usr/local/openresty/lualib/workmesh_waf/
COPY runtime/modsecurity.conf /etc/workmesh-waf/modsecurity.conf
COPY runtime/rules/ /etc/workmesh-waf/rules/
COPY runtime/nginx.conf /usr/local/openresty/nginx/conf/nginx.conf
COPY runtime/docker-entrypoint.sh /usr/local/bin/workmesh-waf-entrypoint

RUN mkdir -p /etc/workmesh-waf/crs /opt/workmesh/waf/data/ip-groups /opt/workmesh/waf/cache /opt/workmesh/waf/logs /opt/workmesh/waf/generated \
    /usr/local/openresty/nginx/conf/conf.d /usr/local/openresty/nginx/conf/stream.d /usr/local/openresty/nginx/conf/default /usr/local/openresty/nginx/conf/ssl \
    && wget -qO /tmp/crs.tar.gz https://github.com/coreruleset/coreruleset/archive/refs/tags/v${CRS_VERSION}.tar.gz \
    && tar -xzf /tmp/crs.tar.gz --strip-components=1 -C /etc/workmesh-waf/crs \
    && cp /etc/workmesh-waf/crs/crs-setup.conf.example /etc/workmesh-waf/crs/crs-setup.conf \
    && rm -f /tmp/crs.tar.gz

# Install the WorkMesh CRS settings after extraction so the runtime file is
# the configuration actually included by /etc/workmesh-waf/modsecurity.conf.
COPY runtime/crs-setup.conf /etc/workmesh-waf/crs/crs-setup.conf

RUN printf '{"openresty":"1.27.1.2","modsecurity":"%s","crs":"%s","runtime":"workmesh-waf-lua"}\n' "$MODSECURITY_VERSION" "$CRS_VERSION" > /opt/workmesh/waf/runtime-version.json \
    && printf '# WorkMesh 默认模式\nSecRuleEngine DetectionOnly\n' > /opt/workmesh/waf/generated/modsecurity-mode.conf \
    && : > /opt/workmesh/waf/generated/custom-rules.conf \
    && printf '# WorkMesh standard CRS rules enabled\nInclude /etc/workmesh-waf/crs/crs-setup.conf\nInclude /etc/workmesh-waf/crs/rules/*.conf\n' > /opt/workmesh/waf/generated/standard-rules.conf \
    && : > /opt/workmesh/waf/generated/global-rules.conf \
    && chmod +x /usr/local/bin/workmesh-waf-entrypoint

RUN /usr/local/openresty/nginx/sbin/nginx -t

HEALTHCHECK --interval=30s --timeout=5s --retries=3 CMD /usr/local/openresty/nginx/sbin/nginx -t

ENTRYPOINT ["/usr/local/bin/workmesh-waf-entrypoint"]
CMD ["/usr/local/openresty/nginx/sbin/nginx", "-g", "daemon off;"]
