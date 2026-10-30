import { useEffect, useState } from "react";
import { BrainCircuit, ChevronRight, CircleUserRound, FlaskConical, History, House, LineChart, LoaderCircle, Menu, X } from "lucide-react";
import { trackedSymbols } from "./api";

export type Page = "overview" | "analysis" | "backtest" | "history" | "portfolio";

export const pages: [string, Page, typeof House][] = [
  ["总览", "overview", House], ["AI 研判", "analysis", BrainCircuit], ["策略回测", "backtest", FlaskConical],
  ["历史反应", "history", History], ["我的持仓", "portfolio", CircleUserRound],
];

function Logo() {
  return <div className="logo"><LineChart size={20} strokeWidth={1.75} /><strong>宏观透镜</strong><span>MacroLens</span></div>;
}

// 美股常规交易时段（纽约时间），不考虑节假日
function marketStatus(now: Date): string {
  const parts = Object.fromEntries(new Intl.DateTimeFormat("en-US", { timeZone: "America/New_York", weekday: "short", hour: "numeric", minute: "numeric", hourCycle: "h23" })
    .formatToParts(now).map(p => [p.type, p.value]));
  if (parts.weekday === "Sat" || parts.weekday === "Sun") return "美股休市";
  const m = Number(parts.hour) * 60 + Number(parts.minute);
  if (m >= 4 * 60 && m < 9 * 60 + 30) return "美股盘前";
  if (m >= 9 * 60 + 30 && m < 16 * 60) return "美股盘中";
  if (m >= 16 * 60 && m < 20 * 60) return "美股盘后";
  return "美股休市";
}

function useNow(ms: number) {
  const [now, setNow] = useState(() => new Date());
  useEffect(() => { const t = setInterval(() => setNow(new Date()), ms); return () => clearInterval(t); }, [ms]);
  return now;
}

export function Header({ page, setPage }: { page: Page; setPage: (p: Page) => void }) {
  const [open, setOpen] = useState(false);
  const now = useNow(1000);
  const status = marketStatus(now);
  const clock = now.toLocaleTimeString("zh-CN", { timeZone: "Asia/Shanghai", hour12: false });
  return (
    <header className="site-header">
      <div className="header-inner">
        <button className="logo-button" onClick={() => setPage("overview")}><Logo /></button>
        <nav className="desktop-nav">
          {pages.map(([label, value]) => <button className={page === value ? "active" : ""} onClick={() => setPage(value)} key={value}>{label}</button>)}
        </nav>
        <div className="header-side"><span>北京时间 {clock}</span><i className={status === "美股盘中" ? "open" : ""}>{status}</i></div>
        <button className="menu-button" aria-label="打开菜单" onClick={() => setOpen(!open)}>{open ? <X /> : <Menu />}</button>
      </div>
      {open && <div className="mobile-menu">{pages.map(([label, value]) => <button onClick={() => { setPage(value); setOpen(false); }} key={value}>{label}<ChevronRight size={18} /></button>)}</div>}
    </header>
  );
}

export function MobileNav({ page, setPage }: { page: Page; setPage: (p: Page) => void }) {
  const items = pages.filter(([, v]) => v !== "history");
  return <nav className="mobile-tabs">{items.map(([name, value, Icon]) => <button className={page === value ? "active" : ""} onClick={() => setPage(value)} key={value}><Icon size={21} strokeWidth={1.75} /><span>{name}</span></button>)}</nav>;
}

export function SymbolPicker({ value, onChange }: { value: string; onChange: (s: string) => void }) {
  return <div className="segmented symbol-picker">{trackedSymbols.map(([sym, label]) => <button className={value === sym ? "active" : ""} onClick={() => onChange(sym)} key={sym}>{label}</button>)}</div>;
}

export function Loading({ text = "加载中…" }: { text?: string }) {
  return <div className="state-box"><LoaderCircle className="spin" size={20} /><span>{text}</span></div>;
}

export function ErrorBox({ text, onRetry }: { text: string; onRetry?: () => void }) {
  return <div className="state-box error"><span>{text}</span>{onRetry && <button onClick={onRetry}>重试</button>}</div>;
}

// 把走势点缩放进 160×50 画布，上下各留 6px
export function sparkPath(points: number[]) {
  const min = Math.min(...points), max = Math.max(...points), span = max - min || 1;
  return points.map((v, i) => `${i ? "L" : "M"}${(2 + (i / (points.length - 1)) * 156).toFixed(1)} ${(44 - ((v - min) / span) * 38).toFixed(1)}`).join(" ");
}

export function Sparkline({ points, up }: { points: number[] | null; up: boolean }) {
  if (!points || points.length < 2) return null;
  return <svg viewBox="0 0 160 50" className={up ? "spark up" : "spark down"} aria-hidden="true"><path d={sparkPath(points)} /></svg>;
}

export function Disclaimer() {
  return <p className="disclaimer">本内容由模型和规则自动生成，仅供信息参考，不构成投资建议</p>;
}
