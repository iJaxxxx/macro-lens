// Package market 从外部行情源拉取报价。
package market

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"
)

// Quote 是某个品种的最新报价。
type Quote struct {
	Price     float64
	PrevClose float64
	Points    []float64 // 最近一个交易日的走势，按时间先后排列
	Time      time.Time // 最新价对应的时间
}

// Bar 是一根日线，只保留收盘价，归因和回测都只用收盘价。
type Bar struct {
	Date  string // 纽约时间的交易日，如 "2026-10-29"
	Close float64
}

// Yahoo 通过 Yahoo Finance 的 chart 接口拉行情。这是非官方接口，无需 key，但可能被限流。
type Yahoo struct {
	BaseURL string
	HTTP    *http.Client
}

func NewYahoo() *Yahoo {
	return &Yahoo{
		BaseURL: "https://query1.finance.yahoo.com",
		HTTP:    &http.Client{Timeout: 8 * time.Second},
	}
}

// 走势只取最后一个点之前这段时间内的点。股指一天交易 6.5 小时，期货接近 23 小时，
// 20 小时足够覆盖一个交易日，又不会混进前一天的股指数据。
const sessionWindow = 20 * 60 * 60

// 日线按纽约时间划分交易日，期货夜盘的时间戳也能落到正确的那一天
var newYork = mustLoad("America/New_York")

func mustLoad(name string) *time.Location {
	loc, err := time.LoadLocation(name)
	if err != nil {
		panic(err)
	}
	return loc
}

type chartResult struct {
	Meta struct {
		RegularMarketPrice float64 `json:"regularMarketPrice"`
		RegularMarketTime  int64   `json:"regularMarketTime"`
		PreviousClose      float64 `json:"previousClose"`
	} `json:"meta"`
	Timestamp  []int64 `json:"timestamp"`
	Indicators struct {
		Quote []struct {
			Close []*float64 `json:"close"`
		} `json:"quote"`
	} `json:"indicators"`
}

type chartResponse struct {
	Chart struct {
		Result []chartResult `json:"result"`
		Error  *struct {
			Description string `json:"description"`
		} `json:"error"`
	} `json:"chart"`
}

func (y *Yahoo) chart(ctx context.Context, symbol, rng, interval string) (chartResult, error) {
	u := fmt.Sprintf("%s/v8/finance/chart/%s?range=%s&interval=%s", y.BaseURL, url.PathEscape(symbol), rng, interval)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return chartResult{}, err
	}
	// 不带浏览器 UA 时 Yahoo 经常直接返回 429
	req.Header.Set("User-Agent", "Mozilla/5.0")
	resp, err := y.HTTP.Do(req)
	if err != nil {
		return chartResult{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return chartResult{}, fmt.Errorf("yahoo %s: status %d", symbol, resp.StatusCode)
	}

	var body chartResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return chartResult{}, fmt.Errorf("yahoo %s: %w", symbol, err)
	}
	if body.Chart.Error != nil {
		return chartResult{}, fmt.Errorf("yahoo %s: %s", symbol, body.Chart.Error.Description)
	}
	if len(body.Chart.Result) == 0 {
		return chartResult{}, fmt.Errorf("yahoo %s: empty result", symbol)
	}
	return body.Chart.Result[0], nil
}

func (r chartResult) closes() []*float64 {
	if len(r.Indicators.Quote) == 0 {
		return nil
	}
	return r.Indicators.Quote[0].Close
}

func (y *Yahoo) Quote(ctx context.Context, symbol string) (Quote, error) {
	// 用 5 天范围而不是 1 天：周末或期货刚开盘时 1 天范围里可能一个点都没有
	r, err := y.chart(ctx, symbol, "5d", "15m")
	if err != nil {
		return Quote{}, err
	}
	closes := r.closes()
	var points []float64
	if n := min(len(r.Timestamp), len(closes)); n > 0 {
		start := r.Timestamp[n-1] - sessionWindow
		for i := range n {
			if r.Timestamp[i] >= start && closes[i] != nil {
				points = append(points, *closes[i])
			}
		}
	}

	q := Quote{Price: r.Meta.RegularMarketPrice, PrevClose: r.Meta.PreviousClose, Points: points}
	if r.Meta.RegularMarketTime > 0 {
		q.Time = time.Unix(r.Meta.RegularMarketTime, 0)
	}
	if q.Price == 0 {
		return Quote{}, fmt.Errorf("yahoo %s: missing price", symbol)
	}
	if q.PrevClose == 0 && len(points) > 0 {
		q.PrevClose = points[0]
	}
	return q, nil
}

// History 返回最近五年的日线，按日期先后排列。同一天有多个点时保留最后一个。
func (y *Yahoo) History(ctx context.Context, symbol string) ([]Bar, error) {
	r, err := y.chart(ctx, symbol, "5y", "1d")
	if err != nil {
		return nil, err
	}
	closes := r.closes()
	var bars []Bar
	for i := range min(len(r.Timestamp), len(closes)) {
		if closes[i] == nil || *closes[i] == 0 {
			continue
		}
		date := time.Unix(r.Timestamp[i], 0).In(newYork).Format(time.DateOnly)
		if n := len(bars); n > 0 && bars[n-1].Date == date {
			bars[n-1].Close = *closes[i]
			continue
		}
		bars = append(bars, Bar{Date: date, Close: *closes[i]})
	}
	if len(bars) == 0 {
		return nil, fmt.Errorf("yahoo %s: no daily bars", symbol)
	}
	return bars, nil
}
