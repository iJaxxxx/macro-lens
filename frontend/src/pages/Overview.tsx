import { useState } from "react";
import { ChevronRight, Clock3, Sparkles } from "lucide-react";
import { Attribution, FactorTile, MarketAsset, Scorecard, pct, plainPct, q, useApi } from "../api";
import { ErrorBox, Loading, Sparkline, SymbolPicker } from "../components";

// 宏观日历还没有接入数据源，先展示示例
const events = [
  ["周三 22:30", "EIA 原油库存", "高"],
  ["周四 20:30", "美国 CPI", "高"],
  ["周五 22:00", "密歇根消费者信心", "中"],
];

function timeOf(iso?: string) {
  return iso ? new Date(iso).toLocaleTimeString("zh-CN", { timeZone: "Asia/Shanghai", hour12: false, hour: "2-digit", minute: "2-digit" }) : "";
}

function MarketCards() {
  const { data, error } = useApi<MarketAsset[]>("/api/assets", 30_000);
  if (!data) return error ? <ErrorBox text={error} /> : <Loading />;
  return (
    <section className="market-scroll">
      {data.map(item => <article className={`market-card ${item.up ? "card-up" : "card-down"}`} key={item.symbol}>
        <div className="market-top"><div><strong>{item.name}</strong><span>{item.code}</span></div><em className={item.up ? "gain" : "loss"}>{item.change}</em></div>
        <div className="market-value">{item.price}</div>
        <div className="market-meta">{item.live ? <><i className="live-dot" />实时 · {timeOf(item.updatedAt)}</> : "示例数据"}</div>
        <Sparkline points={item.points} up={item.up} />
      </article>)}
    </section>
  );
}

function AttributionCard() {
  const [symbol, setSymbol] = useState("^GSPC");
  const { data: info, error, reload } = useApi<Attribution>(`/api/attribution?symbol=${q(symbol)}`, 60_000);
  return (
    <section className={`white-card attribution-card ${info && info.total < 0 ? "move-down" : "move-up"}`}>
      <div className="card-heading">
        <div><p>今日涨跌归因{info && ` · ${info.date}`}</p><h2>{info ? `${info.name} ${pct(info.total)}` : "—"}</h2></div>
        <SymbolPicker value={symbol} onChange={setSymbol} />
      </div>
      {!info ? (error ? <ErrorBox text={error} onRetry={reload} /> : <Loading text="正在计算因子归因…" />) :
        <div className="attribution-grid">
          <div className="factor-bars">{info.rows.map(r => <div className={`factor-bar ${r.contribution >= 0 ? "pos" : "neg"}`} key={r.name}>
            <div><span>{r.name}<small>{r.factor}{r.move && ` ${r.move}`}</small></span><strong>{pct(r.contribution)}</strong></div>
            <div className="bar-track"><i style={{ width: `${Math.max(1, r.share * 100)}%` }} /></div>
          </div>)}</div>
          <div className="insight"><div className="insight-icon"><Sparkles size={20} /></div><div>
            <h3>{info.title}</h3><p>{info.summary}</p>
            <span>{info.source === "llm" ? "由 Claude 解读" : "规则生成"} · 近 {info.window} 个交易日多因子回归 · 模型解释度 R² {plainPct(info.r2, 0)}</span>
          </div></div>
        </div>}
    </section>
  );
}

function Factors() {
  const [period, setPeriod] = useState<"今日" | "近 5 日">("今日");
  const { data } = useApi<FactorTile[]>("/api/factors", 60_000);
  return (
    <section className="section-block">
      <div className="section-heading"><div><h2>宏观因子</h2><p>影响资产定价的关键变量 · 实时</p></div><div className="small-segment">{(["今日", "近 5 日"] as const).map(x => <button className={period === x ? "active" : ""} onClick={() => setPeriod(x)} key={x}>{x}</button>)}</div></div>
      {!data ? <Loading /> :
        <div className="factor-grid">{data.map(f => {
          const dir = period === "今日" ? f.dayDir : f.weekDir;
          return <article className={`white-card factor-tile trend-${dir}`} key={f.symbol}><span>{f.name}</span><strong>{f.value}</strong><p>{period === "今日" ? f.day : f.week}</p></article>;
        })}</div>}
    </section>
  );
}

function ScoreCard() {
  const { data } = useApi<Scorecard>("/api/scorecard", 300_000);
  return (
    <div className="white-card score-card">
      <p>AI 预测成绩单</p>
      <strong>{data ? Math.round(data.rate * 100) : "—"}<span>%</span></strong>
      <div className="score-track"><i style={{ width: `${data ? data.rate * 100 : 0}%` }} /></div>
      <span>{data ? `近 ${data.days} 个交易日次日方向命中 ${data.hits}/${data.total}` : "近 30 个交易日方向命中率"}</span>
      {data && <div className="score-items">{data.items.map(i => <div key={i.symbol}><span>{i.name}</span><em>{Math.round(i.rate * 100)}%</em></div>)}</div>}
    </div>
  );
}

export default function Overview({ goCalendar }: { goCalendar: () => void }) {
  return (
    <main className="page">
      <section className="hero">
        <p className="eyebrow">今日市场</p>
        <h1>不只看涨跌，更看懂原因。</h1>
        <p>宏观因子如何影响美股、黄金与原油，一目了然。</p>
      </section>
      <MarketCards />
      <AttributionCard />
      <Factors />
      <section className="overview-bottom">
        <div className="white-card events-card"><div className="card-title-row"><h2>即将发生 <small className="tag">示例</small></h2><button onClick={goCalendar}>历史反应 <ChevronRight size={16} /></button></div>{events.map(([time, name, level]) => <div className="event-row" key={name}><Clock3 size={18} /><div><strong>{name}</strong><span>{time}</span></div><i>{level}重要性</i></div>)}</div>
        <ScoreCard />
      </section>
    </main>
  );
}
