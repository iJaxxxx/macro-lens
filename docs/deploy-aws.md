# 部署到 AWS EC2（上手指南）

整体思路：租一台 Linux 云服务器（EC2），本地把前端打包、Go 编译成 Linux 可执行文件，
上传到服务器，用 systemd 让它常驻运行。浏览器访问服务器公网 IP 即可看到网站。

```
浏览器 ──http:80──▶ EC2 (Amazon Linux)
                     └─ /opt/macro-lens/macro-lens   Go 程序：/api/* 接口 + 托管前端
                     └─ /opt/macro-lens/dist/        前端打包产物
```

## 第 1 步：注册 AWS 账号（自己操作）

1. 打开 <https://aws.amazon.com/free> → 创建账号。需要邮箱、手机验证和一张信用卡（会预授权一小笔钱）。
2. 注册时选 **Free plan（免费计划）**。2025-07-15 之后注册的新账号是“赠送额度 + 6 个月”模式，
   具体以注册页面显示为准。
3. 登录后，右上角账号菜单 → **Billing and Cost Management → Budgets**，建一个月预算（比如 5 美元），
   超出会发邮件提醒。新手最容易踩的坑就是忘关资源被扣钱。

## 第 2 步：创建 EC2 实例（自己操作）

控制台顶部搜索 **EC2** → 右上角选区域（推荐 **Asia Pacific (Tokyo)** 或 **Singapore**）→ **Launch instance**：

| 选项 | 填什么 |
| --- | --- |
| Name | `macro-lens` |
| AMI | **Amazon Linux 2023**（登录用户名是 `ec2-user`） |
| Instance type | **t3.micro**（带 “Free tier eligible” 标记；发布脚本按 x86/amd64 编译） |
| Key pair | Create new key pair → 名字 `macro-lens`，类型 RSA，格式 `.pem` → 浏览器会下载 `macro-lens.pem`，**只能下载这一次** |
| Network settings | 勾选 **Allow SSH traffic from → My IP**；勾选 **Allow HTTP traffic from the internet** |
| Storage | 默认 8 GiB 即可 |

点 **Launch instance**，等状态变成 Running，在实例详情里复制 **Public IPv4 address**。

把密钥放好并收紧权限（SSH 要求私钥只有自己能读）：

```bash
mv ~/Downloads/macro-lens.pem ~/.ssh/ && chmod 400 ~/.ssh/macro-lens.pem
```

## 第 3 步：连上服务器

```bash
ssh -i ~/.ssh/macro-lens.pem ec2-user@<公网IP>
```

第一次会问 `Are you sure you want to continue connecting`，输入 `yes`。看到 `[ec2-user@ip-... ~]$` 就是连上了，
可以试试 `uname -a`、`free -h`，输入 `exit` 退出。

连不上时依次检查：实例是否 Running；安全组 22 端口来源是不是你当前的 IP（换了网络要重新改）；
用户名是不是 `ec2-user`；密钥权限是不是 400。

## 第 4 步：发布

本地需要 Go 和 Node 20+（Homebrew 装的 `node@22` 可以这样临时用：`export PATH="/opt/homebrew/opt/node@22/bin:$PATH"`）。

```bash
HOST=<公网IP> KEY=~/.ssh/macro-lens.pem ./deploy/deploy.sh
```

脚本做了四件事：`npm run build` 打包前端 → `GOOS=linux GOARCH=amd64 go build` 交叉编译 →
`scp` 上传 → 安装 systemd 服务并重启。最后打印 `{"status":"ok"}` 就成功了，浏览器打开 `http://<公网IP>`。

以后改了代码，再跑一次同样的命令就是重新发布。

## 第 5 步：日常运维

```bash
ssh -i ~/.ssh/macro-lens.pem ec2-user@<公网IP>
sudo systemctl status macro-lens     # 看是否在运行
sudo journalctl -u macro-lens -f     # 实时看日志（Go 里 log.Printf 的输出）
sudo systemctl restart macro-lens    # 重启
```

不用的时候在 EC2 控制台 **Stop instance** 可以省钱；**Terminate** 是彻底删除，删了就回不来。
注意：Stop 再 Start 后公网 IP 会变，需要固定 IP 可以申请 Elastic IP。

## 用 GitHub Actions 自动发布

[`.github/workflows/deploy.yml`](../.github/workflows/deploy.yml) 会在每次 push 到 `main` 后，在 GitHub 的服务器上
跑后端测试，然后执行同一个 `deploy/deploy.sh`。本地网络连不上服务器（比如公司 VPN 拦截了 SSH）时也能发布。
也可以在仓库的 **Actions → Deploy → Run workflow** 手动触发。

第一次使用前需要配置两处（自己操作）：

1. **GitHub Secrets**：仓库 **Settings → Secrets and variables → Actions → New repository secret**，添加：

   | Name | Value |
   | --- | --- |
   | `EC2_HOST` | 服务器公网 IP |
   | `EC2_SSH_KEY` | `macro-lens.pem` 的完整内容，可以用 `pbcopy < ~/.ssh/macro-lens.pem` 复制 |

2. **安全组放行 GitHub**：GitHub 的服务器 IP 不固定，需要把 22 端口的入站规则来源改成 `0.0.0.0/0`（Anywhere-IPv4）。
   EC2 只允许密钥登录，没有私钥的人连上 22 端口也登不进去；介意的话可以只在发布时临时放开。

发布结果在仓库的 **Actions** 页面查看，失败时点进去能看到每一步的日志。

## 配置登录和大模型（可选）

密钥类配置不进仓库，放在服务器上的 `/etc/macro-lens.env`，systemd 启动时读取：

```bash
ssh -i ~/.ssh/macro-lens.pem ec2-user@<公网IP>
sudo tee /etc/macro-lens.env > /dev/null <<'ENV'
GOOGLE_CLIENT_ID=xxxx.apps.googleusercontent.com
ALLOWED_EMAILS=you@gmail.com
ANTHROPIC_API_KEY=sk-ant-...
ENV
sudo chmod 600 /etc/macro-lens.env
sudo systemctl restart macro-lens
```

- 不需要的项删掉即可：没有 `GOOGLE_CLIENT_ID` 时登录关闭，没有 `ANTHROPIC_API_KEY` 时 AI 研判用规则引擎。
- Google 登录要求网站是 HTTPS 域名（见下一节），用公网 IP 直接访问时登录按钮会报错。
- 用户的交易记录存在 `/opt/macro-lens/data`，重新发布不会覆盖。建议定期备份这个目录。

## 后续可以做的

- 绑定域名 + HTTPS（Route 53 或其他域名商 + Caddy / Nginx 自动签证书）。Google 登录依赖这一步；
  反向代理需要转发 `X-Forwarded-Proto`，后端据此给会话 Cookie 加上 `Secure`
