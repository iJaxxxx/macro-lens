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

type chartResponse struct {
	Chart struct {
		Result []struct {
			Meta struct {
				RegularMarketPrice float64 `json:"regularMarketPrice"`
				PreviousClose      float64 `json:"previousClose"`
			} `json:"meta"`
			Timestamp  []int64 `json:"timestamp"`
			Indicators struct {
				Quote []struct {
					Close []*float64 `json:"close"`
				} `json:"quote"`
			} `json:"indicators"`
		} `json:"result"`
		Error *struct {
			Description string `json:"description"`
		} `json:"error"`
	} `json:"chart"`
}

func (y *Yahoo) Quote(ctx context.Context, symbol string) (Quote, error) {
	// 用 5 天范围而不是 1 天：周末或期货刚开盘时 1 天范围里可能一个点都没有
	u := fmt.Sprintf("%s/v8/finance/chart/%s?range=5d&interval=15m", y.BaseURL, url.PathEscape(symbol))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return Quote{}, err
	}
	// 不带浏览器 UA 时 Yahoo 经常直接返回 429
	req.Header.Set("User-Agent", "Mozilla/5.0")
	resp, err := y.HTTP.Do(req)
	if err != nil {
		return Quote{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return Quote{}, fmt.Errorf("yahoo %s: status %d", symbol, resp.StatusCode)
	}

	var body chartResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return Quote{}, fmt.Errorf("yahoo %s: %w", symbol, err)
	}
	if body.Chart.Error != nil {
		return Quote{}, fmt.Errorf("yahoo %s: %s", symbol, body.Chart.Error.Description)
	}
	if len(body.Chart.Result) == 0 {
		return Quote{}, fmt.Errorf("yahoo %s: empty result", symbol)
	}
	r := body.Chart.Result[0]

	var closes []*float64
	if len(r.Indicators.Quote) > 0 {
		closes = r.Indicators.Quote[0].Close
	}
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
	if q.Price == 0 {
		return Quote{}, fmt.Errorf("yahoo %s: missing price", symbol)
	}
	if q.PrevClose == 0 && len(points) > 0 {
		q.PrevClose = points[0]
	}
	return q, nil
}
