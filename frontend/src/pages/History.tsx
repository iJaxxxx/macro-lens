import { useMemo, useState } from "react";
import { Bell, CalendarDays, Check, ChevronDown, RotateCcw } from "lucide-react";

// 事件预期值需要付费数据源，本页暂为示例数据
const historyRows = [
  ["2026-09-11", "2.6%", "2.9%", "+0.3%", "+0.42%", "+0.86%", "-1.12%"],
  ["2026-08-14", "2.7%", "2.7%", "0.0%", "-0.18%", "+0.31%", "+0.44%"],
  ["2026-07-11", "3.1%", "3.3%", "+0.2%", "+0.63%", "+1.08%", "-0.72%"],
  ["2026-06-12", "3.4%", "3.2%", "-0.2%", "-0.36%", "-0.52%", "+1.24%"],
  ["2026-05-15", "3.5%", "3.7%", "+0.2%", "+0.28%", "+0.91%", "-0.38%"],
];

export default function History() {
  const [condition, setCondition] = useState("高于预期");
  const [event, setEvent] = useState("美国 CPI");
  const [range, setRange] = useState("近 5 年");
  const [reminded, setReminded] = useState(false);
  const [sortDesc, setSortDesc] = useState(true);
  const rows = useMemo(() => sortDesc ? historyRows : [...historyRows].reverse(), [sortDesc]);
  return (
    <main className="page">
      <section className="hero compact"><p className="eyebrow">历史反应</p><h1>过去，能告诉我们什么？</h1><p>对比宏观事件发生后，不同资产的典型表现。</p></section>
      <p className="note">本页为示例数据：宏观事件的市场预期值需要付费数据源，接入后替换。</p>
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
