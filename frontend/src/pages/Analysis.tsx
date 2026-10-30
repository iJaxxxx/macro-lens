import { useState } from "react";
import { ArrowDown, ArrowUp, Bot, Gavel, ShieldCheck, Users } from "lucide-react";
import { AgentReport, SiteConfig, money, plainPct, q, useApi } from "../api";
import { Disclaimer, ErrorBox, Loading, SymbolPicker } from "../components";

const tone = (decision: string) => (["买入", "增持", "看多"].includes(decision) ? "bullish" : ["卖出", "减持", "看空"].includes(decision) ? "bearish" : "neutral");

function Step({ n, icon: Icon, title, children }: { n: number; icon: typeof Bot; title: string; children: React.ReactNode }) {
  return <section className="agent-step"><header><span>{n}</span><Icon size={17} /><h2>{title}</h2></header>{children}</section>;
}

function Report({ r }: { r: AgentReport }) {
  return <>
    <section className={`white-card verdict ${tone(r.final.decision)}`}>
      <div>
        <p>{r.name} · 数据截至 {r.date} · 最新价 {money(r.price)}</p>
        <h2>{r.final.decision}</h2>
        <span>{r.final.summary}</span>
      </div>
      <div className="verdict-side">
        <small>置信度</small><strong>{Math.round(r.final.confidence * 100)}%</strong>
        <div className="score-track"><i style={{ width: `${r.final.confidence * 100}%` }} /></div>
        <em>{r.mode === "llm" ? `Claude 多智能体 · ${r.model}` : "规则引擎"} · {new Date(r.generatedAt).toLocaleString("zh-CN", { hour12: false })}</em>
      </div>
    </section>
    {r.note && <p className="note">{r.note}</p>}

    <Step n={1} icon={Users} title="分析师团队">
      <div className="analyst-grid">{r.analysts.map(a => <article className={`white-card analyst ${tone(a.stance)}`} key={a.role}>
        <div><strong>{a.role}</strong><em>{a.stance} {a.score >= 0 ? "+" : ""}{a.score.toFixed(2)}</em></div>
        <p>{a.summary}</p>
        <ul>{a.points.map(p => <li key={p}>{p}</li>)}</ul>
      </article>)}</div>
    </Step>

    <Step n={2} icon={Bot} title="多空辩论">
      <div className="debate">
        {([["多方研究员", r.bull, "bullish", ArrowUp], ["空方研究员", r.bear, "bearish", ArrowDown]] as const).map(([name, arg, cls, Icon]) =>
          <article className={`white-card ${cls}`} key={name}><div><Icon size={16} /><strong>{name}</strong></div><p>{arg.summary}</p><ul>{(arg.points ?? []).map(p => <li key={p}>{p}</li>)}</ul></article>)}
      </div>
    </Step>

    <Step n={3} icon={Gavel} title="交易员方案">
      <article className={`white-card plan ${tone(r.trader.decision)}`}><strong>{r.trader.decision}</strong><em>置信度 {Math.round(r.trader.confidence * 100)}%</em><p>{r.trader.rationale}</p></article>
    </Step>

    <Step n={4} icon={ShieldCheck} title="风控审核">
      <article className="white-card risk">
        <div className="risk-stats">
          <div><small>建议仓位</small><strong>{plainPct(r.risk.position, 0)}</strong></div>
          <div><small>止损</small><strong className="loss">{money(r.risk.stopLoss)}</strong></div>
          <div><small>止盈</small><strong className="gain">{money(r.risk.takeProfit)}</strong></div>
        </div>
        <ul>{(r.risk.notes ?? []).map(n => <li key={n}>{n}</li>)}</ul>
      </article>
    </Step>
  </>;
}

export default function Analysis() {
  const [symbol, setSymbol] = useState("^GSPC");
  const { data: cfg } = useApi<SiteConfig>("/api/config");
  const { data, error, reload } = useApi<AgentReport>(`/api/analysis?symbol=${q(symbol)}`);
  return (
    <main className="page analysis-page">
      <section className="hero compact"><p className="eyebrow">AI 研判</p><h1>一支虚拟投研团队，替你把关。</h1>
        <p>参考 TradingAgents 的分工：分析师 → 多空辩论 → 交易员 → 风控，所有判断都基于实时指标。</p></section>
      <div className="toolbar"><SymbolPicker value={symbol} onChange={setSymbol} /></div>
      {data ? <Report r={data} /> : error ? <ErrorBox text={error} onRetry={reload} /> :
        <Loading text={cfg?.llm ? "智能体团队正在研判，大模型模式首次约需 1 分钟…" : "正在研判…"} />}
      <Disclaimer />
    </main>
  );
}
