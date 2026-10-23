# 宏观透镜 MacroLens

美股 / 黄金 / 原油宏观分析网站，基于 Figma Make 设计「美股分析网站设计」实现。

页面：总览（涨跌归因、宏观因子、事件日历）、历史反应（事件后资产表现统计）、AI 复盘（每日宏观简报）。目前数据为设计稿内的示例数据。

## 目录结构

```
frontend/   React 19 + Vite + Tailwind CSS v4 前端
backend/    Go 后端（标准库 net/http）
  cmd/server/      入口
  internal/api/    路由与接口
```

## 本地开发

后端（默认端口 8080，可用 `PORT` 覆盖）：

```bash
cd backend && go run ./cmd/server
```

前端（需要 Node 20+，开发服务器会把 `/api` 代理到 8080）：

```bash
cd frontend && npm install && npm run dev
```

## 接口

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| GET | `/api/health` | 健康检查 |
| GET | `/api/assets` | 首页行情卡片数据 |

## 部署

发布到 AWS EC2 的完整步骤见 [docs/deploy-aws.md](docs/deploy-aws.md)，配置好后一条命令发布：

```bash
HOST=<服务器公网 IP> KEY=~/.ssh/macro-lens.pem ./deploy/deploy.sh
```
