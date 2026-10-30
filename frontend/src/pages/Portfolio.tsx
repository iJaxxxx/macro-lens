import { useEffect, useRef, useState } from "react";
import { LogOut, Plus, Trash2 } from "lucide-react";
import { Portfolio as PortfolioData, SiteConfig, Transaction, User, api, money, pct, signedMoney, useApi } from "../api";
import { ErrorBox, Loading } from "../components";

declare global {
  interface Window {
    google?: {
      accounts: {
        id: {
          initialize(opts: { client_id: string; callback: (r: { credential: string }) => void }): void;
          renderButton(el: HTMLElement, opts: Record<string, unknown>): void;
        };
      };
    };
  }
}

let gisScript: Promise<void> | null = null;
function loadGoogleScript() {
  gisScript ??= new Promise((resolve, reject) => {
    const s = document.createElement("script");
    s.src = "https://accounts.google.com/gsi/client";
    s.async = true;
    s.onload = () => resolve();
    s.onerror = () => { gisScript = null; reject(new Error("无法加载 Google 登录组件")); };
    document.head.appendChild(s);
  });
  return gisScript;
}

function SignIn({ clientId, onLogin }: { clientId: string; onLogin: (u: User) => void }) {
  const ref = useRef<HTMLDivElement>(null);
  const [error, setError] = useState<string | null>(null);
  useEffect(() => {
    loadGoogleScript().then(() => {
      if (!window.google || !ref.current) return;
      window.google.accounts.id.initialize({
        client_id: clientId,
        callback: ({ credential }) => api<User>("/api/auth/google", { method: "POST", body: JSON.stringify({ credential }) })
          .then(onLogin).catch(e => setError(e.message)),
      });
      window.google.accounts.id.renderButton(ref.current, { theme: "filled_black", size: "large", text: "signin_with", shape: "rectangular", locale: "zh_CN" });
    }).catch(e => setError(e.message));
  }, [clientId, onLogin]);
  return <div className="signin"><div ref={ref} />{error && <p className="form-error">{error}</p>}</div>;
}

const today = () => new Date().toLocaleDateString("en-CA", { timeZone: "Asia/Shanghai" });
const emptyForm = () => ({ symbol: "VOO", side: "buy" as "buy" | "sell", quantity: "", price: "", fee: "0", date: today(), note: "" });

function TransactionForm({ onSaved }: { onSaved: (p: PortfolioData) => void }) {
  const [form, setForm] = useState(emptyForm);
  const [error, setError] = useState<string | null>(null);
  const [saving, setSaving] = useState(false);
  const set = (k: keyof ReturnType<typeof emptyForm>) => (e: React.ChangeEvent<HTMLInputElement | HTMLSelectElement>) => setForm({ ...form, [k]: e.target.value });

  const submit = async (e: React.FormEvent) => {
    e.preventDefault();
    setSaving(true); setError(null);
    try {
      const p = await api<PortfolioData>("/api/portfolio/transactions", {
        method: "POST",
        body: JSON.stringify({ ...form, quantity: Number(form.quantity), price: Number(form.price), fee: Number(form.fee || 0) }),
      });
      onSaved(p);
      setForm({ ...emptyForm(), symbol: form.symbol, date: form.date });
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setSaving(false);
    }
  };

  return (
    <form className="white-card tx-form" onSubmit={submit}>
      <h2>记一笔</h2>
      <div className="form-grid">
        <label>代码<input value={form.symbol} onChange={set("symbol")} placeholder="VOO / GC=F" required /></label>
        <label>方向<select value={form.side} onChange={set("side")}><option value="buy">买入</option><option value="sell">卖出</option></select></label>
        <label>数量<input type="number" step="any" min="0" value={form.quantity} onChange={set("quantity")} required /></label>
        <label>成交价<input type="number" step="any" min="0" value={form.price} onChange={set("price")} required /></label>
        <label>手续费<input type="number" step="any" min="0" value={form.fee} onChange={set("fee")} /></label>
        <label>日期<input type="date" value={form.date} max={today()} onChange={set("date")} required /></label>
        <label className="wide">备注<input value={form.note} onChange={set("note")} maxLength={200} placeholder="可选" /></label>
      </div>
      {error && <p className="form-error">{error}</p>}
      <button className="primary-action" disabled={saving}><Plus size={16} /> {saving ? "保存中…" : "保存"}</button>
    </form>
  );
}

function Holdings({ p }: { p: PortfolioData }) {
  const open = p.holdings.filter(h => h.quantity > 0);
  const t = p.totals;
  const cls = (x: number) => (x >= 0 ? "gain" : "loss");
  return <>
    <section className="stat-row">
      <div className="white-card stat"><small>持仓市值</small><strong>{money(t.marketValue)}</strong></div>
      <div className="white-card stat"><small>浮动盈亏</small><strong className={cls(t.unrealized)}>{signedMoney(t.unrealized)}</strong><span>{t.costBasis > 0 ? pct(t.unrealized / t.costBasis) : ""}</span></div>
      <div className="white-card stat"><small>今日盈亏</small><strong className={cls(t.dayChange)}>{signedMoney(t.dayChange)}</strong></div>
      <div className="white-card stat"><small>已实现盈亏</small><strong className={cls(t.realized)}>{signedMoney(t.realized)}</strong></div>
    </section>
    <section className="white-card detail-card">
      <div className="card-title-row"><h2>当前持仓</h2><p>移动加权平均成本 · 行情 30 秒刷新</p></div>
      {open.length === 0 ? <p className="empty">还没有持仓，在下方记录第一笔交易。</p> :
        <div className="detail-scroll"><div className="detail-table holdings-table">
          <div className="detail-row head"><span>代码</span><span>数量</span><span>成本价</span><span>现价</span><span>市值</span><span>浮动盈亏</span><span>今日</span></div>
          {open.map(h => <div className="detail-row" key={h.symbol}>
            <span><strong>{h.symbol}</strong>{h.name && h.name !== h.symbol && <small> {h.name}</small>}</span>
            <span>{h.quantity}</span><span>{money(h.avgCost)}</span>
            <span>{money(h.price)}{!h.live && <small title="拉不到报价，按成本价估值"> *</small>}</span>
            <span>{money(h.marketValue)}</span>
            <span className={cls(h.unrealized)}>{signedMoney(h.unrealized)} · {pct(h.unrealizedPct)}</span>
            <span className={cls(h.dayChange)}>{signedMoney(h.dayChange)}</span>
          </div>)}
        </div></div>}
    </section>
  </>;
}

function Transactions({ txs, onChange }: { txs: Transaction[]; onChange: (p: PortfolioData) => void }) {
  const [error, setError] = useState<string | null>(null);
  const remove = async (id: string) => {
    if (!confirm("删除这条交易记录？")) return;
    try {
      onChange(await api<PortfolioData>(`/api/portfolio/transactions/${id}`, { method: "DELETE" }));
      setError(null);
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    }
  };
  return (
    <section className="white-card detail-card">
      <div className="card-title-row"><h2>交易记录</h2><p>{txs.length} 笔</p></div>
      {error && <p className="form-error">{error}</p>}
      {txs.length === 0 ? <p className="empty">暂无记录</p> :
        <div className="detail-scroll"><div className="detail-table tx-table">
          <div className="detail-row head"><span>日期</span><span>代码</span><span>方向</span><span>数量</span><span>价格</span><span>手续费</span><span>备注</span><span /></div>
          {txs.map(t => <div className="detail-row" key={t.id}>
            <span>{t.date}</span><span>{t.symbol}</span>
            <span className={t.side === "buy" ? "gain" : "loss"}>{t.side === "buy" ? "买入" : "卖出"}</span>
            <span>{t.quantity}</span><span>{money(t.price)}</span><span>{money(t.fee)}</span><span className="note-cell">{t.note}</span>
            <span><button className="icon-button" aria-label="删除" onClick={() => remove(t.id)}><Trash2 size={14} /></button></span>
          </div>)}
        </div></div>}
    </section>
  );
}

function Dashboard({ user, onLogout }: { user: User; onLogout: () => void }) {
  const { data, error, reload } = useApi<PortfolioData>("/api/portfolio", 30_000);
  const [local, setLocal] = useState<PortfolioData | null>(null);
  useEffect(() => { if (data) setLocal(data); }, [data]);
  const p = local ?? data;
  return <>
    <section className="white-card user-bar">
      {user.picture && <img src={user.picture} alt="" referrerPolicy="no-referrer" />}
      <div><strong>{user.name || user.email}</strong><span>{user.email}</span></div>
      <button className="secondary-action" onClick={onLogout}><LogOut size={15} /> 退出</button>
    </section>
    {!p ? (error ? <ErrorBox text={error} onRetry={reload} /> : <Loading />) : <>
      <Holdings p={p} />
      <TransactionForm onSaved={setLocal} />
      <Transactions txs={p.transactions} onChange={setLocal} />
    </>}
  </>;
}

export default function Portfolio() {
  const { data: cfg } = useApi<SiteConfig>("/api/config");
  const [user, setUser] = useState<User | null | undefined>(undefined);
  useEffect(() => {
    api<User>("/api/me").then(setUser).catch(() => setUser(null));
  }, []);
  const logout = () => api("/api/auth/logout", { method: "POST" }).finally(() => setUser(null));

  return (
    <main className="page portfolio-page">
      <section className="hero compact"><p className="eyebrow">我的持仓</p><h1>个人投资记录</h1><p>使用 Google 账号登录后记录交易，数据只对你自己可见。</p></section>
      {user === undefined ? <Loading /> : user ? <Dashboard user={user} onLogout={logout} /> :
        <section className="white-card login-card">
          <h2>登录后查看你的持仓</h2>
          <p>我们只读取 Google 账号的邮箱、昵称和头像，用来识别你的身份。</p>
          {!cfg ? <Loading /> : cfg.googleClientId ? <SignIn clientId={cfg.googleClientId} onLogin={setUser} /> :
            <p className="form-error">服务器尚未配置 Google 登录（GOOGLE_CLIENT_ID），请联系管理员。</p>}
        </section>}
    </main>
  );
}
