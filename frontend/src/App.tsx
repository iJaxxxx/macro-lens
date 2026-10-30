import { useEffect, useState } from "react";
import { Header, MobileNav, Page, pages } from "./components";
import Overview from "./pages/Overview";
import Analysis from "./pages/Analysis";
import Backtest from "./pages/Backtest";
import History from "./pages/History";
import Portfolio from "./pages/Portfolio";

// 当前页面记在地址栏的 hash 里，刷新或分享链接时停留在同一页
function pageFromHash(): Page {
  const h = location.hash.slice(1);
  return pages.some(([, v]) => v === h) ? (h as Page) : "overview";
}

export default function App() {
  const [page, setPageState] = useState<Page>(pageFromHash);
  useEffect(() => {
    const onHash = () => setPageState(pageFromHash());
    addEventListener("hashchange", onHash);
    return () => removeEventListener("hashchange", onHash);
  }, []);
  const setPage = (p: Page) => { location.hash = p; window.scrollTo(0, 0); };

  const body = {
    overview: <Overview goCalendar={() => setPage("history")} />,
    analysis: <Analysis />,
    backtest: <Backtest />,
    history: <History />,
    portfolio: <Portfolio />,
  }[page];
  return <><Header page={page} setPage={setPage} />{body}<MobileNav page={page} setPage={setPage} /></>;
}
