import { useState } from "react";
import { Backtest as BacktestData, Strategy, pct, plainPct, q, useApi } from "../api";
import { Disclaimer, ErrorBox, Loading, SymbolPicker } from "../components";

// 绘图区用 0~100 的坐标，按容器拉伸；文字和圆点用 HTML 叠加，避免跟着 SVG 缩放变形
const xPct = (i: number, len: number) => (i / Math.max(1, len - 1)) * 100;

// 净值曲线：选中的策略 vs 买入持有基准，鼠标悬停显示两条线在该点的净值
function EquityChart({ strategy, benchmark, from, to }: { strategy: Strategy; benchmark: Strategy; from: string; to: string }) {
  const [hover, setHover] = useState<number | null>(null);
  const series = [benchmark.equity, strategy.equity];
  const all = series.flat();
  const lo = Math.min(...all), hi = Math.max(...all), span = hi - lo || 1;
  const n = Math.max(...series.map(s => s.length));
  const yPct = (v: number) => 4 + (1 - (v - lo) / span) * 92;
  const path = (s: number[]) => s.map((v, i) => `${i ? "L" : "M"}${xPct(i, s.length).toFixed(2)} ${yPct(v).toFixed(2)}`).join(" ");
  const ticks = [hi, lo + span / 2, lo];
  const at = (s: number[], i: number) => s[Math.round((i / Math.max(1, n - 1)) * (s.length - 1))];
  const showBench = strategy.key !== benchmark.key;

  const onMove = (e: React.PointerEvent<HTMLDivElement>) => {
    const rect = e.currentTarget.getBoundingClientRect();
    const f = (e.clientX - rect.left) / rect.width;
    setHover(Math.max(0, Math.min(n - 1, Math.round(f * (n - 1)))));
  };
  const hx = hover === null ? 0 : xPct(hover, n);

  return (
    <div className="equity-chart">
      <div className="legend"><span className="series-strategy">{strategy.name}</span>{showBench && <span className="series-benchmark">买入持有</span>}</div>
      <div className="chart-body">
        <div className="y-axis">{ticks.map(t => <span key={t} style={{ top: `${yPct(t)}%` }}>{t.toFixed(2)}</span>)}</div>
        <div className="plot" onPointerMove={onMove} onPointerLeave={() => setHover(null)} role="img" aria-label={`${strategy.name} 与买入持有的净值曲线`}>
          <svg viewBox="0 0 100 100" preserveAspectRatio="none">
            {ticks.map(t => <line key={t} className="grid" x1={0} x2={100} y1={yPct(t)} y2={yPct(t)} />)}
            {showBench && <path className="line benchmark" d={path(benchmark.equity)} />}
            <path className="line strategy" d={path(strategy.equity)} />
            {hover !== null && <line className="crosshair" x1={hx} x2={hx} y1={0} y2={100} />}
          </svg>
          {hover !== null && <>
            {showBench && <i className="dot benchmark" style={{ left: `${hx}%`, top: `${yPct(at(benchmark.equity, hover))}%` }} />}
            <i className="dot strategy" style={{ left: `${hx}%`, top: `${yPct(at(strategy.equity, hover))}%` }} />
            <div className={`tooltip ${hx > 70 ? "left" : ""}`} style={{ left: `${hx}%` }}>
              <div><i className="series-strategy" />{strategy.name}<strong>{at(strategy.equity, hover).toFixed(3)}</strong></div>
              {showBench && <div><i className="series-benchmark" />买入持有<strong>{at(benchmark.equity, hover).toFixed(3)}</strong></div>}
            </div>
          </>}
        </div>
      </div>
      <div className="x-axis"><span>{from}</span><span>{to}</span></div>
    </div>
  );
}

export default function Backtest() {
  const [symbol, setSymbol] = useState("^GSPC");
  const [selected, setSelected] = useState("sma_cross");
  const { data, error, reload } = useApi<BacktestData>(`/api/backtest?symbol=${q(symbol)}`);
  const strategy = data?.strategies.find(s => s.key === selected) ?? data?.strategies[0];
  const benchmark = data?.strategies.find(s => s.key === "buyhold");
  return (
    <main className="page">
      <section className="hero compact"><p className="eyebrow">策略回测</p><h1>规则说了算，数据来检验。</h1>
        <p>近三年日线逐日回测，只做多或空仓，每次换仓扣除 5 个基点成本。</p></section>
      <div className="toolbar"><SymbolPicker value={symbol} onChange={setSymbol} /></div>
      {!data ? (error ? <ErrorBox text={error} onRetry={reload} /> : <Loading text="正在回测…" />) : <>
        <section className="white-card path-card">
          <div className="card-title-row"><div><h2>净值曲线 · {data.name}</h2><p>{data.from} 至 {data.to} · 起点净值为 1 · {strategy?.description}</p></div></div>
          {strategy && benchmark && <EquityChart strategy={strategy} benchmark={benchmark} from={data.from} to={data.to} />}
        </section>
        <section className="white-card detail-card">
          <div className="card-title-row"><h2>策略对比</h2><p>点击一行查看净值曲线</p></div>
          <div className="detail-scroll"><div className="detail-table bt-table">
            <div className="detail-row head"><span>策略</span><span>年化收益</span><span>总收益</span><span>夏普</span><span>最大回撤</span><span>胜率</span><span>交易次数</span><span>持仓时间</span><span>当前信号</span></div>
            {data.strategies.map(s => <button className={`detail-row ${s.key === strategy?.key ? "selected" : ""}`} onClick={() => setSelected(s.key)} key={s.key}>
              <span>{s.name}</span>
              <span className={s.cagr >= 0 ? "gain" : "loss"}>{pct(s.cagr, 1)}</span>
              <span className={s.totalReturn >= 0 ? "gain" : "loss"}>{pct(s.totalReturn, 1)}</span>
              <span>{s.sharpe.toFixed(2)}</span>
              <span className="loss">{plainPct(-s.maxDrawdown)}</span>
              <span>{s.key === "buyhold" ? "—" : plainPct(s.winRate, 0)}</span>
              <span>{s.trades}</span>
              <span>{plainPct(s.exposure, 0)}</span>
              <span className={s.position > 0 ? "gain" : ""}>{s.position > 0 ? "持有" : "空仓"}</span>
            </button>)}
          </div></div>
        </section>
      </>}
      <Disclaimer />
    </main>
  );
}
