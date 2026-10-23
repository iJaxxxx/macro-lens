#!/usr/bin/env bash
# 一键发布到 EC2：本地打包前后端 -> 上传 -> 重启服务。
# 用法：HOST=<服务器公网 IP> KEY=<.pem 路径> ./deploy/deploy.sh
set -euo pipefail

: "${HOST:?请设置 HOST，例如 HOST=13.112.0.1}"
: "${KEY:?请设置 KEY，例如 KEY=~/.ssh/macro-lens.pem}"
# t3.micro 是 x86 架构；如果换成 t4g 系列（Graviton），改成 arm64
GOARCH="${GOARCH:-amd64}"

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
OUT="$ROOT/backend/bin"
SSH=(ssh -i "$KEY" "ec2-user@$HOST")

echo "==> 打包前端"
(cd "$ROOT/frontend" && npm ci && npm run build)

echo "==> 交叉编译 Go（linux/$GOARCH）"
mkdir -p "$OUT"
(cd "$ROOT/backend" && CGO_ENABLED=0 GOOS=linux GOARCH="$GOARCH" go build -o "$OUT/macro-lens" ./cmd/server)

echo "==> 上传到 $HOST"
"${SSH[@]}" "rm -rf ~/release && mkdir -p ~/release"
scp -i "$KEY" -r "$OUT/macro-lens" "$ROOT/frontend/dist" "$ROOT/deploy/macro-lens.service" "ec2-user@$HOST:~/release/"

echo "==> 安装并重启服务"
"${SSH[@]}" 'set -e
sudo mkdir -p /opt/macro-lens
sudo rm -rf /opt/macro-lens/dist
sudo cp -r ~/release/macro-lens ~/release/dist /opt/macro-lens/
sudo chown -R ec2-user:ec2-user /opt/macro-lens
sudo cp ~/release/macro-lens.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable macro-lens
sudo systemctl restart macro-lens
sleep 1
curl -fsS http://localhost/api/health'

echo
echo "==> 发布完成：http://$HOST"
