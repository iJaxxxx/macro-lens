# 宏观透镜 MacroLens

美股 / 黄金 / 原油宏观分析网站，基于 Figma Make 设计「美股分析网站设计」实现。

页面：

| 页面 | 内容 | 数据 |
| --- | --- | --- |
| 总览 | 标普 500 / 纳斯达克 / 道琼斯 / VOO / 黄金 / 原油实时行情（30 秒刷新）、今日涨跌归因、宏观因子、预测成绩单 | 实时 |
| AI 研判 | 仿 [TradingAgents](https://github.com/TauricResearch/TradingAgents) 的多智能体流程：技术 / 宏观 / 情绪分析师 → 多空辩论 → 交易员 → 风控 | 实时 |
| 策略回测 | 近三年日线回测买入持有、均线交叉、200 日趋势、动量、MACD、RSI 均值回归 | 实时 |
| 历史反应 | 宏观事件发生后的资产表现 | 示例（事件预期值需要付费数据源） |
| 我的持仓 | Google 账号登录后记录交易，按移动加权成本计算持仓和盈亏 | 实时 |

行情来自 Yahoo Finance 的非官方接口（无需 key）。

### 归因是怎么算的

对每个品种，取最近 120 个交易日的日收益，对几个宏观因子的日变化做 OLS 回归：
美股看 10 年期美债收益率（基点变化）、美元指数、VIX、铜；黄金看美债收益率、美元、VIX；
原油看美元、铜、标普 500、VIX。用估出的 beta 乘以当天各因子的变化，得到每个因子对当天涨跌的贡献，
剩下解释不了的部分归为「其他」。R² 是回归的解释度。解读文字默认按规则生成，配置了 Claude 后由模型改写。

### AI 研判的两种模式

- **规则引擎**（默认）：把均线、MACD、RSI、波动率分位、因子 beta × 近 5 日因子变化等指标翻译成各角色观点，
  按 技术 40% / 宏观 35% / 情绪 25% 加权给出决策，按年化 15% 波动目标定仓位。
- **Claude**（设置 `ANTHROPIC_API_KEY` 后启用）：七次模型调用分别扮演三位分析师、多空研究员、交易员和风控经理，
  输入是后端算好的同一套指标，结果按品种缓存 6 小时。

## 目录结构

```
frontend/   React 19 + Vite + Tailwind CSS v4 前端
  src/pages/           各页面
backend/    Go 后端（标准库 net/http）
  cmd/server/          入口
  internal/api/        路由、缓存与接口
  internal/market/     Yahoo 行情（报价、日线）
  internal/quant/      技术指标、多因子回归归因、策略回测
  internal/agents/     多智能体研判（规则引擎 / Claude）
  internal/llm/        Claude 调用封装
  internal/auth/       Google ID Token 校验、会话 Cookie
  internal/portfolio/  交易记录存储与持仓计算
```

## 本地开发

后端（默认端口 8080）：

```bash
cd backend && go run ./cmd/server
```

前端（需要 Node 20+，开发服务器会把 `/api` 代理到 8080）：

```bash
cd frontend && npm install && npm run dev
```

### 环境变量

| 变量 | 说明 |
| --- | --- |
| `PORT` | 监听端口，默认 8080 |
| `STATIC_DIR` | 前端打包产物目录，生产环境由 Go 托管前端 |
| `DATA_DIR` | 交易记录和会话密钥的存放目录，默认 `./data` |
| `GOOGLE_CLIENT_ID` | Google OAuth 客户端 ID，不设置则关闭登录 |
| `ALLOWED_EMAILS` | 可选，逗号分隔的邮箱白名单，只允许这些账号登录 |
| `SESSION_SECRET` | 可选，会话签名密钥；不设置时自动生成并保存在 `DATA_DIR/session.key` |
| `ANTHROPIC_API_KEY` | 可选，设置后 AI 研判和归因解读由 Claude 生成 |
| `ANTHROPIC_MODEL` | 可选，默认 `claude-opus-5-5` |
| `YAHOO_BASE_URL` | 可选，行情接口地址，可指向代理或本地模拟服务 |

### 配置 Google 登录

1. 打开 [Google Cloud Console → API 和服务 → 凭据](https://console.cloud.google.com/apis/credentials)，
   创建「OAuth 客户端 ID」，类型选「Web 应用」。
2. 「已获授权的 JavaScript 来源」填网站地址，例如 `https://macro-lens.example.com`；本地开发加上 `http://localhost:5173`。
   **Google 只允许 localhost 使用 http，线上必须是 HTTPS 域名，直接用 IP 访问时无法登录。**
3. 把得到的客户端 ID 设为 `GOOGLE_CLIENT_ID`。

后端校验 Google ID Token 的签名、签发方、受众和过期时间，通过后签发 30 天有效的 HttpOnly 会话 Cookie。
交易记录按 Google 账号 ID 分文件存放，用户之间互相不可见。

## 接口

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| GET | `/api/health` | 健康检查 |
| GET | `/api/config` | 前端需要的配置（Google 客户端 ID、是否启用大模型） |
| GET | `/api/assets` | 行情卡片 |
| GET | `/api/factors` | 宏观因子 |
| GET | `/api/attribution?symbol=` | 今日涨跌归因 |
| GET | `/api/analysis?symbol=` | 多智能体研判 |
| GET | `/api/backtest?symbol=` | 策略回测 |
| GET | `/api/scorecard` | 技术信号近 30 日次日方向命中率 |
| POST | `/api/auth/google` | 用 Google ID Token 登录 |
| POST | `/api/auth/logout` | 退出 |
| GET | `/api/me` | 当前用户 |
| GET | `/api/portfolio` | 持仓和交易记录（需登录） |
| POST | `/api/portfolio/transactions` | 新增交易（需登录） |
| DELETE | `/api/portfolio/transactions/{id}` | 删除交易（需登录） |

`symbol` 取 `^GSPC`、`^IXIC`、`^DJI`、`VOO`、`GC=F`、`CL=F` 之一。

## 部署

发布到 AWS EC2 的完整步骤见 [docs/deploy-aws.md](docs/deploy-aws.md)，配置好后一条命令发布：

```bash
HOST=<服务器公网 IP> KEY=~/.ssh/macro-lens.pem ./deploy/deploy.sh
```
