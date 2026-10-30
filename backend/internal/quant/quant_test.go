package quant

import (
	"fmt"
	"math"
	"math/rand/v2"
	"testing"

	"github.com/iJaxxxx/macro-lens/backend/internal/market"
)

func near(a, b, eps float64) bool { return math.Abs(a-b) <= eps }

func TestOLSRecoversCoefficients(t *testing.T) {
	rng := rand.New(rand.NewPCG(1, 2))
	var x [][]float64
	var y []float64
	for range 200 {
		a, b := rng.NormFloat64(), rng.NormFloat64()
		x = append(x, []float64{1, a, b})
		y = append(y, 0.5+2*a-3*b)
	}
	beta, r2, err := OLS(x, y)
	if err != nil {
		t.Fatal(err)
	}
	if !near(beta[0], 0.5, 1e-9) || !near(beta[1], 2, 1e-9) || !near(beta[2], -3, 1e-9) || !near(r2, 1, 1e-9) {
		t.Errorf("beta = %v, r2 = %v", beta, r2)
	}
}

func TestOLSCollinear(t *testing.T) {
	if _, _, err := OLS([][]float64{{1, 2}, {1, 2}, {1, 2}}, []float64{1, 2, 3}); err == nil {
		t.Fatal("want error on collinear factors")
	}
}

// 构造一个资产，日收益 = -0.5 × 美元涨跌 - 0.001 × 收益率基点变化，检查归因能拆出来
func TestAttribute(t *testing.T) {
	rng := rand.New(rand.NewPCG(3, 4))
	var asset, usd, yield []market.Bar
	pa, pu, py := 100.0, 100.0, 4.0
	for i := range 160 {
		date := fmt.Sprintf("d%03d", i)
		ru, dy := rng.NormFloat64()*0.005, rng.NormFloat64()*0.05
		if i > 0 {
			pu *= 1 + ru
			py += dy
			pa *= 1 - 0.5*ru - 0.001*dy*100 + rng.NormFloat64()*0.0001
		}
		asset = append(asset, market.Bar{Date: date, Close: pa})
		usd = append(usd, market.Bar{Date: date, Close: pu})
		yield = append(yield, market.Bar{Date: date, Close: py})
	}
	// 资产多一天没有因子数据，应被对齐掉
	asset = append(asset, market.Bar{Date: "zzz", Close: 1})

	a, err := Attribute(asset, []FactorInput{{"美元", FactorReturn, usd}, {"10年期美债", FactorDiffBP, yield}}, 120)
	if err != nil {
		t.Fatal(err)
	}
	if a.Date != "d159" || a.Window != 120 {
		t.Errorf("date = %s, window = %d", a.Date, a.Window)
	}
	if !near(a.Contributions[0].Beta, -0.5, 0.02) || !near(a.Contributions[1].Beta, -0.001, 0.0001) {
		t.Errorf("betas = %v, %v", a.Contributions[0].Beta, a.Contributions[1].Beta)
	}
	if a.R2 < 0.95 {
		t.Errorf("r2 = %v", a.R2)
	}
	sum := a.Residual
	share := a.ResidualShare
	for _, c := range a.Contributions {
		sum += c.Contribution
		share += c.Share
	}
	if !near(sum, a.Total, 1e-12) || !near(share, 1, 1e-9) {
		t.Errorf("contributions don't add up: sum=%v total=%v share=%v", sum, a.Total, share)
	}
}

func TestIndicators(t *testing.T) {
	up := make([]float64, 100)
	for i := range up {
		up[i] = float64(100 + i)
	}
	if r := RSI(up, 14); r != 100 {
		t.Errorf("RSI of rising series = %v", r)
	}
	if s := TechnicalScore(up); s <= 0.5 {
		t.Errorf("score of rising series = %v", s)
	}
	if dd := MaxDrawdown([]float64{1, 1.2, 0.9, 1.3}); !near(dd, 0.25, 1e-12) {
		t.Errorf("max drawdown = %v", dd)
	}
	snap := TakeSnapshot(make([]string, 100), up)
	if snap.FromHigh != 0 || snap.SMA200 != 0 {
		t.Errorf("snapshot = %+v", snap)
	}
}

func TestBacktestBuyHold(t *testing.T) {
	closes := []float64{100, 110, 99, 121}
	r := Backtest(Strategies[0], closes, 0)
	// 第一天建仓当天扣一次成本，之后一直持有
	want := (1.1 - tradeCost) * 1.1
	if !near(r.TotalReturn+1, want, 1e-12) || r.Trades != 1 || r.Exposure != 1 {
		t.Errorf("result = %+v, want total %v", r, want-1)
	}
}

func TestHitRate(t *testing.T) {
	up := make([]float64, 120)
	for i := range up {
		up[i] = float64(100 + i)
	}
	hits, total := HitRate(up, 30)
	if total != 30 || hits != 30 {
		t.Errorf("hits = %d / %d", hits, total)
	}
}
