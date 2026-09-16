镜像构建阶段从固定 tag 下载 OWASP CRS 4.14.0 到 /etc/workmesh-waf/crs，本目录存放 WorkMesh 离线兜底规则。运行时只允许通过 WorkMesh 控制面生成站点 JSON 和 modsecurity-mode.conf，不要在容器内手工改规则。
