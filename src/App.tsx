import { useMemo, useState } from "react";
import {
  Bell, BellRing, CalendarDays, Check, ChevronDown, ChevronRight, CircleUserRound,
  Clock3, FileText, House, LineChart, Menu, RotateCcw, Sparkles, X,
} from "lucide-react";

type Page = "overview" | "history" | "review";
type Asset = "黄金" | "原油" | "美股";

const assets = [
  { name: "标普 500", code: "S&P 500", price: "5,842.3", change: "+0.62%", up: true, path: "M2 43 C25 36 28 44 50 31 S77 35 96 21 S126 22 158 7" },
  { name: "现货黄金", code: "XAU / USD", price: "2,687.4", change: "+1.84%", up: true, path: "M2 44 C23 45 30 34 51 37 S77 24 96 27 S119 12 158 6" },
  { name: "WTI 原油", code: "CL1!", price: "68.92", change: "-2.15%", up: false, path: "M2 8 C25 16 34 9 53 22 S80 17 101 31 S130 26 158 43" },
];

const factors = [
  ["10年期美债实际利率", "1.82%", "↓ 下降 7bp", "down"],
  ["美元指数 DXY", "103.2", "↓ 下降 0.41%", "down"],
  ["通胀预期", "2.31%", "→ 保持稳定", "flat"],
  ["VIX 恐慌指数", "16.4", "↑ 上升 3.2%", "up"],
  ["EIA 原油库存", "+3.2M", "↑ 高于预期", "up"],
  ["上金所溢价", "+$12.5", "↑ 较伦敦现货", "up"],
];

const attribution: Record<Asset, { total: string; title: string; copy: string; rows: [string, number, string][] }> = {
  黄金: { total: "+1.84%", title: "为什么黄金今天涨了？", copy: "实际利率快速下行，降低了持有黄金的机会成本。美元走弱进一步放大涨幅，避险需求也提供了温和支撑。", rows: [["实际利率下行", 60, "+1.10%"], ["美元走弱", 25, "+0.46%"], ["避险情绪", 8, "+0.15%"], ["其他", 7, "+0.13%"]] },
  原油: { total: "-2.15%", title: "为什么原油今天跌了？", copy: "库存意外增加是主要压力，炼厂开工率下降进一步强化供需趋松预期。美元走弱仅抵消了部分跌幅。", rows: [["库存增加", 54, "-1.16%"], ["需求预期", 28, "-0.60%"], ["美元影响", 11, "-0.24%"], ["其他", 7, "-0.15%"]] },
  美股: { total: "+0.62%", title: "为什么美股今天涨了？", copy: "实际利率回落缓解了科技股估值压力，半导体与大型科技股领涨。风险偏好改善，但成交量仍低于近期均值。", rows: [["利率回落", 48, "+0.30%"], ["盈利预期", 31, "+0.19%"], ["风险偏好", 15, "+0.09%"], ["其他", 6, "+0.04%"]] },
};

const events = [
  ["周三 22:30", "EIA 原油库存", "高"],
  ["周四 20:30", "美国 CPI", "高"],
  ["周五 22:00", "密歇根消费者信心", "中"],
];

const historyRows = [
  ["2026-09-11", "2.6%", "2.9%", "+0.3%", "+0.42%", "+0.86%", "-1.12%"],
  ["2026-08-14", "2.7%", "2.7%", "0.0%", "-0.18%", "+0.31%", "+0.44%"],
  ["2026-07-11", "3.1%", "3.3%", "+0.2%", "+0.63%", "+1.08%", "-0.72%"],
  ["2026-06-12", "3.4%", "3.2%", "-0.2%", "-0.36%", "-0.52%", "+1.24%"],
  ["2026-05-15", "3.5%", "3.7%", "+0.2%", "+0.28%", "+0.91%", "-0.38%"],
];

const reviewDates = ["10月18日", "10月17日", "10月16日", "10月15日", "10月14日", "10月13日", "10月12日", "10月11日"];

function Logo() {
  return <div className="logo"><LineChart size={20} strokeWidth={1.75} /><strong>宏观透镜</strong><span>MacroLens</span></div>;
}

function Header({ page, setPage }: { page: Page; setPage: (p: Page) => void }) {
  const [open, setOpen] = useState(false);
  const nav: [string, Page][] = [["总览", "overview"], ["历史反应", "history"], ["AI 复盘", "review"]];
  return (
    <header className="site-header">
      <div className="header-inner">
        <button className="logo-button" onClick={() => setPage("overview")}><Logo /></button>
        <nav className="desktop-nav">
          {nav.map(([label, value]) => <button className={page === value ? "active" : ""} onClick={() => setPage(value)} key={label}>{label}</button>)}
        </nav>
        <div className="header-side"><span>北京时间 21:30</span><i>美股盘前</i></div>
        <button className="menu-button" aria-label="打开菜单" onClick={() => setOpen(!open)}>{open ? <X /> : <Menu />}</button>
      </div>
      {open && <div className="mobile-menu">{nav.map(([label, value]) => <button onClick={() => { setPage(value); setOpen(false); }} key={label}>{label}<ChevronRight size={18} /></button>)}</div>}
    </header>
  );
}

function MobileNav({ page, setPage }: { page: Page; setPage: (p: Page) => void }) {
  const items: [string, Page, typeof House][] = [["总览", "overview", House], ["日历", "history", CalendarDays], ["复盘", "review", FileText], ["我的", "overview", CircleUserRound]];
  return <nav className="mobile-tabs">{items.map(([name, value, Icon]) => <button className={page === value ? "active" : ""} onClick={() => setPage(value)} key={name}><Icon size={21} strokeWidth={1.75} /><span>{name}</span></button>)}</nav>;
}

function Sparkline({ path, up }: { path: string; up: boolean }) {
  return <svg viewBox="0 0 160 50" className={up ? "spark up" : "spark down"}><path d={path} /></svg>;
}

function Overview() {
  const [asset, setAsset] = useState<Asset>("美股");
  const [period, setPeriod] = useState("今日");
  const info = attribution[asset];
  return (
    <main className="page">
      <section className="hero">
        <p className="eyebrow">今日市场</p>
        <h1>不只看涨跌，更看懂原因。</h1>
        <p>宏观因子如何影响美股、黄金与原油，一目了然。</p>
      </section>

      <section className="market-scroll">
        {assets.map((item) => <article className={`market-card ${item.name === "标普 500" ? "us-market" : ""}`} key={item.name}>
          <div className="market-top"><div><strong>{item.name}</strong><span>{item.code}</span></div><em className={item.up ? "gain" : "loss"}>{item.change}</em></div>
          <div className="market-value">{item.price}</div><Sparkline path={item.path} up={item.up} />
        </article>)}
      </section>

      <section className={`white-card attribution-card ${asset === "美股" ? "us-market" : ""} ${info.total.startsWith("-") ? "move-down" : "move-up"}`}>
        <div className="card-heading"><div><p>今日涨跌归因</p><h2>{asset} {info.total}</h2></div><div className="segmented">{(["美股", "黄金", "原油"] as Asset[]).map(x => <button className={asset === x ? "active" : ""} onClick={() => setAsset(x)} key={x}>{x}</button>)}</div></div>
        <div className="attribution-grid">
          <div className="factor-bars">{info.rows.map(([name, width, value]) => <div className="factor-bar" key={name}><div><span>{name}</span><strong>{value}</strong></div><div className="bar-track"><i style={{ width: `${width}%` }} /></div></div>)}</div>
          <div className="insight"><div className="insight-icon"><Sparkles size={20} /></div><div><h3>{info.title}</h3><p>{info.copy}</p><span>由 AI 生成 · 模型解释度 93%</span></div></div>
        </div>
      </section>

      <section className="section-block">
        <div className="section-heading"><div><h2>宏观因子</h2><p>影响资产定价的关键变量</p></div><div className="small-segment">{["今日", "本周"].map(x => <button className={period === x ? "active" : ""} onClick={() => setPeriod(x)} key={x}>{x}</button>)}</div></div>
        <div className="factor-grid">{factors.map(([name, value, note, direction]) => <article className={`white-card factor-tile trend-${period === "今日" ? direction : "flat"}`} key={name}><span>{name}</span><strong>{value}</strong><p>{period === "今日" ? note : "→ 本周趋势稳定"}</p></article>)}</div>
      </section>

      <section className="overview-bottom">
        <div className="white-card events-card"><div className="card-title-row"><h2>即将发生</h2><button>查看日历 <ChevronRight size={16} /></button></div>{events.map(([time, name, level]) => <div className="event-row" key={name}><Clock3 size={18} /><div><strong>{name}</strong><span>{time}</span></div><i>{level}重要性</i></div>)}</div>
        <div className="white-card score-card"><p>AI 预测成绩单</p><strong>64<span>%</span></strong><div className="score-track"><i /></div><span>近 30 天方向命中率</span></div>
      </section>
    </main>
  );
}

function History() {
  const [condition, setCondition] = useState("高于预期");
  const [event, setEvent] = useState("美国 CPI");
  const [range, setRange] = useState("近 5 年");
  const [reminded, setReminded] = useState(false);
  const [sortDesc, setSortDesc] = useState(true);
  const rows = useMemo(() => sortDesc ? historyRows : [...historyRows].reverse(), [sortDesc]);
  return (
    <main className="page">
      <section className="hero compact"><p className="eyebrow">历史反应</p><h1>过去，能告诉我们什么？</h1><p>对比宏观事件发生后，不同资产的典型表现。</p></section>
      <section className="white-card filters">
        <label>事件<select value={event} onChange={e => setEvent(e.target.value)}><option>美国 CPI</option><option>美联储议息</option><option>非农就业</option></select><ChevronDown size={16} /></label>
        <div><span>实际值与预期</span><div className="segmented">{["全部", "高于预期", "低于预期"].map(x => <button className={condition === x ? "active" : ""} onClick={() => setCondition(x)} key={x}>{x}</button>)}</div></div>
        <label>范围<select value={range} onChange={e => setRange(e.target.value)}><option>近 5 年</option><option>近 3 年</option><option>近 1 年</option></select><ChevronDown size={16} /></label>
        <strong>28 <span>次样本</span></strong>
      </section>
      <div className="history-layout">
        <div>
          <section className="white-card summary-card"><div className="card-title-row"><div><h2>统计汇总</h2><p>{event} · {condition}</p></div></div>
            <div className="summary-table"><div className="summary-row head"><span>资产</span><span>1 小时后</span><span>1 天后</span><span>5 天后</span></div>
              {[["标普 500", "+0.18% · 57%", "+0.32% · 61%", "+0.74% · 68%"], ["现货黄金", "+0.41% · 64%", "+0.67% · 71%", "+1.12% · 75%"], ["WTI 原油", "-0.22% · 43%", "-0.46% · 39%", "-0.81% · 36%"]].map(row => <div className={`summary-row ${row[0] === "标普 500" ? "us-market" : ""}`} key={row[0]}>{row.map((x, i) => <span className={i && x.startsWith("-") ? "cool" : i ? "warm" : ""} key={x}>{x}</span>)}</div>)}
            </div>
          </section>
          <section className="white-card path-card"><div className="card-title-row"><div><h2>事件后平均路径</h2><p>28 次事件后的标普 500 表现</p></div></div><div className="path-chart"><svg viewBox="0 0 700 240">{[55,110,165].map(y => <line key={y} x1="35" x2="680" y1={y} y2={y} />)}{[1,2,3,4,5,6,7].map((_, i) => <path key={i} className="ghost" d={`M35 120 C130 ${75 + i * 10},220 ${140 - i * 6},310 ${95 + i * 5} S510 ${80 + i * 8},680 ${60 + i * 5}`} />)}<path className="average" d="M35 120 C150 112,205 102,310 106 S510 77,680 62" /><line className="event-line" x1="35" x2="35" y1="25" y2="205" /></svg><div><span>事件时刻</span><span>1天</span><span>2天</span><span>3天</span><span>4天</span><span>5天</span></div></div></section>
        </div>
        <aside className="white-card next-card"><CalendarDays size={24} /><p>下一次 CPI</p><h2>10月24日</h2><span>周四 · 20:30</span><div><small>市场预期</small><strong>2.4%</strong></div><button className={reminded ? "secondary-action" : "primary-action"} onClick={() => setReminded(!reminded)}>{reminded ? <><Check size={18} /> 已设置提醒</> : <><Bell size={18} /> 设置提醒</>}</button></aside>
      </div>
      <section className="white-card detail-card"><div className="card-title-row"><h2>历史事件明细</h2><button onClick={() => setSortDesc(!sortDesc)}>按日期排序 <RotateCcw size={15} /></button></div><div className="detail-scroll"><div className="detail-table"><div className="detail-row head"><span>公布日期</span><span>预期</span><span>实际</span><span>差值</span><span>标普 500</span><span>黄金</span><span>原油</span></div>{rows.map(row => <div className="detail-row" key={row[0]}>{row.map((x, i) => <span className={i === 4 ? x.startsWith("+") ? "us-gain" : "us-loss" : i > 4 ? x.startsWith("+") ? "gain" : "loss" : ""} key={i}>{x}</span>)}</div>)}</div></div></section>
    </main>
  );
}

function Review() {
  const [date, setDate] = useState(reviewDates[0]);
  const [showDates, setShowDates] = useState(false);
  return (
    <main className="page review-page">
      <aside className={`review-dates ${showDates ? "open" : ""}`}><div><strong>每日复盘</strong><button onClick={() => setShowDates(false)}><X size={18} /></button></div>{reviewDates.map((x, i) => <button className={date === x ? "active" : ""} onClick={() => { setDate(x); setShowDates(false); }} key={x}><i className={i === 2 || i === 6 ? "miss" : ""} />{x}<span>{i === 0 ? "今天" : ""}</span></button>)}</aside>
      <article className="brief">
        <button className="date-picker" onClick={() => setShowDates(true)}><CalendarDays size={18} /> 选择日期</button>
        <header><p className="eyebrow">每日宏观简报</p><h1>{date} 宏观复盘</h1><p>利率回落主导跨资产定价：黄金走强，科技股反弹，库存压力拖累原油。</p></header>
        {[
          ["美股", "+0.62%", "科技股领涨，利率压力边际缓和", "实际利率回落缓解高估值板块的贴现率压力，半导体与大型科技股贡献了主要涨幅。", 65],
          ["黄金", "+1.84%", "实际利率与美元形成双重支撑", "实际利率下行贡献主要涨幅，美元指数走弱进一步放大表现，因子结构保持健康。", 84],
          ["原油", "-2.15%", "库存意外累积压制油价", "EIA 库存高于市场预期，炼厂开工率下降令短期供需重新趋松。", 72],
        ].map(([name, move, title, copy, strength]) => <section className={`brief-section ${name === "美股" ? "us-market" : ""} ${String(move).startsWith("-") ? "move-down" : "move-up"}`} key={String(name)}><div><span>{name}</span><strong className={String(move).startsWith("-") ? "loss" : "gain"}>{move}</strong></div><h2>{title}</h2><p>{copy}</p><div className="brief-factor"><span>主要因子贡献</span><div><i style={{ width: `${strength}%` }} /></div><em>{strength}%</em></div></section>)}
        <section className="white-card tomorrow"><h2>明日关注</h2>{events.map(([time, name]) => <div key={name}><span>{time}</span><strong>{name}</strong><ChevronRight size={17} /></div>)}</section>
        <section className="white-card ai-call"><div className="card-title-row"><h2>AI 判断</h2><span>下一交易日</span></div>{[["标普 500", "看涨", 72], ["现货黄金", "看涨", 81], ["WTI 原油", "看跌", 54]].map(([name, call, confidence]) => <div className={`call-row ${name === "标普 500" ? "us-market" : ""} ${call === "看涨" ? "bullish" : call === "看跌" ? "bearish" : "neutral"}`} key={String(name)}><span>{name}</span><strong>{call}</strong><div><i style={{ width: `${confidence}%` }} /></div><em>{confidence}%</em></div>)}</section>
        <footer>本内容仅供信息参考，不构成投资建议</footer>
      </article>
    </main>
  );
}

export default function App() {
  const [page, setPage] = useState<Page>("overview");
  return <><Header page={page} setPage={setPage} />{page === "overview" ? <Overview /> : page === "history" ? <History /> : <Review />}<MobileNav page={page} setPage={setPage} /></>;
}
