import { useCallback, useEffect, useRef, useState } from "react";

export type MarketAsset = {
  symbol: string; group: string; name: string; code: string; price: string; change: string; up: boolean;
  points: number[] | null; live: boolean; updatedAt?: string;
};

export type FactorTile = { symbol: string; name: string; value: string; day: string; week: string; dayDir: string; weekDir: string; available: boolean };

export type Snapshot = {
  price: number; change1d: number; change5d: number; change20d: number; change60d: number; sma20: number; sma50: number; sma200: number;
  rsi14: number; macdHist: number; bollinger: number; vol20: number; volPercentile: number; fromHigh: number; fromLow: number; latestDate: string;
};

export type AttributionRow = { name: string; factor: string; move: string; beta: number; contribution: number; share: number };
export type Attribution = {
  symbol: string; name: string; date: string; total: number; r2: number; window: number; rows: AttributionRow[];
  title: string; summary: string; source: "rule" | "llm"; technical: Snapshot;
};

export type AnalystReport = { role: string; stance: string; score: number; summary: string; points: string[] };
export type Argument = { summary: string; points: string[] };
export type AgentReport = {
  symbol: string; name: string; date: string; mode: "llm" | "rule"; model?: string; note?: string; price: number;
  analysts: AnalystReport[]; bull: Argument; bear: Argument;
  trader: { decision: string; confidence: number; rationale: string };
  risk: { position: number; stopLoss: number; takeProfit: number; notes: string[] };
  final: { decision: string; confidence: number; summary: string };
  generatedAt: string;
};

export type Strategy = {
  key: string; name: string; description: string; totalReturn: number; cagr: number; vol: number; sharpe: number; maxDrawdown: number;
  trades: number; winRate: number; exposure: number; position: number; equity: number[];
};
export type Backtest = { symbol: string; name: string; from: string; to: string; strategies: Strategy[] };

export type Scorecard = { days: number; hits: number; total: number; rate: number; items: { symbol: string; name: string; hits: number; total: number; rate: number }[] };

export type User = { sub: string; email: string; name: string; picture: string };
export type Transaction = { id: string; symbol: string; side: "buy" | "sell"; quantity: number; price: number; fee: number; date: string; note: string };
export type Holding = {
  symbol: string; name: string; quantity: number; avgCost: number; costBasis: number; realized: number; price: number; prevClose: number;
  marketValue: number; unrealized: number; unrealizedPct: number; dayChange: number; live: boolean;
};
export type Portfolio = {
  transactions: Transaction[]; holdings: Holding[];
  totals: { marketValue: number; costBasis: number; unrealized: number; realized: number; dayChange: number };
};
export type SiteConfig = { googleClientId: string; llm: boolean; model?: string };

export class ApiError extends Error {
  constructor(public status: number, message: string) { super(message); }
}

export async function api<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(path, { ...init, headers: { "Content-Type": "application/json", ...init?.headers } });
  const body = await res.json().catch(() => null);
  if (!res.ok) throw new ApiError(res.status, body?.error ?? `请求失败（${res.status}）`);
  return body as T;
}

export type Loadable<T> = { data: T | null; error: string | null; loading: boolean; reload: () => void };

// useApi 拉取接口数据；intervalMs 大于 0 时定时刷新（页面在后台时跳过）。
// 刷新失败时保留上一次的数据，只记录错误。
export function useApi<T>(path: string | null, intervalMs = 0): Loadable<T> {
  const [data, setData] = useState<T | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);
  const pathRef = useRef(path);
  pathRef.current = path;

  const load = useCallback(async (p: string, signal?: AbortSignal) => {
    setLoading(true);
    try {
      const v = await api<T>(p, { signal });
      if (pathRef.current === p) { setData(v); setError(null); }
    } catch (e) {
      if (!signal?.aborted && pathRef.current === p) setError(e instanceof Error ? e.message : String(e));
    } finally {
      if (pathRef.current === p) setLoading(false);
    }
  }, []);

  useEffect(() => {
    setData(null); setError(null);
    if (!path) return;
    const ctrl = new AbortController();
    load(path, ctrl.signal);
    let timer: number | undefined;
    if (intervalMs > 0) timer = window.setInterval(() => { if (!document.hidden) load(path, ctrl.signal); }, intervalMs);
    return () => { ctrl.abort(); window.clearInterval(timer); };
  }, [path, intervalMs, load]);

  return { data, error, loading, reload: () => { if (path) load(path); } };
}

export const pct = (x: number, digits = 2) => `${x >= 0 ? "+" : ""}${(x * 100).toFixed(digits)}%`;
export const plainPct = (x: number, digits = 1) => `${(x * 100).toFixed(digits)}%`;
export const money = (x: number) => x.toLocaleString("en-US", { minimumFractionDigits: 2, maximumFractionDigits: 2 });
export const signedMoney = (x: number) => `${x >= 0 ? "+" : "-"}${money(Math.abs(x))}`;

// 首页跟踪的品种，归因 / 研判 / 回测都用这份列表
export const trackedSymbols: [string, string][] = [
  ["^GSPC", "标普 500"], ["^IXIC", "纳斯达克"], ["^DJI", "道琼斯"], ["VOO", "VOO"], ["GC=F", "黄金"], ["CL=F", "原油"],
];

export const q = (symbol: string) => encodeURIComponent(symbol);
