package quant

import (
	"errors"
	"math"

	"github.com/iJaxxxx/macro-lens/backend/internal/market"
)

// FactorKind 决定因子的日变化怎么算。
type FactorKind int

const (
	FactorReturn FactorKind = iota // 价格类因子用涨跌幅，如美元指数
	FactorDiffBP                   // 收益率类因子用变化的基点数，如 10 年期美债（^TNX 报价单位是 %）
)

type FactorInput struct {
	Name string
	Kind FactorKind
	Bars []market.Bar
}

type Contribution struct {
	Name         string  `json:"name"`
	Beta         float64 `json:"beta"`         // 因子每变动一个单位，资产收益变动多少
	Move         float64 `json:"move"`         // 因子当天的变动（涨跌幅或基点）
	Kind         string  `json:"kind"`         // "return" | "bp"
	Contribution float64 `json:"contribution"` // beta × move，即对资产当天涨跌的贡献
	Share        float64 `json:"share"`        // 贡献绝对值占比，用于画条形图
}

type Attribution struct {
	Date          string         `json:"date"`
	Total         float64        `json:"total"`    // 资产当天涨跌幅
	Residual      float64        `json:"residual"` // 因子解释不了的部分（含截距）
	ResidualShare float64        `json:"residualShare"`
	R2            float64        `json:"r2"`     // 回归窗口内的拟合优度
	Window        int            `json:"window"` // 回归用了多少个交易日
	Contributions []Contribution `json:"contributions"`
}

// align 只保留资产和所有因子都有数据的交易日，返回日期和按列排列的收盘价。
func align(asset []market.Bar, factors []FactorInput) ([]string, [][]float64) {
	maps := make([]map[string]float64, len(factors))
	for i, f := range factors {
		maps[i] = make(map[string]float64, len(f.Bars))
		for _, b := range f.Bars {
			maps[i][b.Date] = b.Close
		}
	}
	var dates []string
	cols := make([][]float64, len(factors)+1)
outer:
	for _, b := range asset {
		row := make([]float64, len(factors))
		for i, m := range maps {
			v, ok := m[b.Date]
			if !ok {
				continue outer
			}
			row[i] = v
		}
		dates = append(dates, b.Date)
		cols[0] = append(cols[0], b.Close)
		for i, v := range row {
			cols[i+1] = append(cols[i+1], v)
		}
	}
	return dates, cols
}

func moves(xs []float64, kind FactorKind) []float64 {
	if kind == FactorReturn {
		return Returns(xs)
	}
	out := make([]float64, len(xs)-1)
	for i := 1; i < len(xs); i++ {
		out[i-1] = (xs[i] - xs[i-1]) * 100
	}
	return out
}

// Attribute 用最近 window 个交易日（不含最新一天）的日收益对因子变化做 OLS 回归，
// 再用估计出的 beta 乘以最新一天的因子变化，拆出各因子对当天涨跌的贡献。
func Attribute(asset []market.Bar, factors []FactorInput, window int) (Attribution, error) {
	dates, cols := align(asset, factors)
	if len(dates) < 40 {
		return Attribution{}, errors.New("对齐后的样本太少")
	}
	y := Returns(cols[0])
	xs := make([][]float64, len(factors))
	for i, f := range factors {
		xs[i] = moves(cols[i+1], f.Kind)
	}
	last := len(y) - 1
	start := max(0, last-window)

	// 设计矩阵：截距 + 各因子
	k := len(factors) + 1
	var rows [][]float64
	var ys []float64
	for t := start; t < last; t++ {
		row := make([]float64, k)
		row[0] = 1
		for i := range factors {
			row[i+1] = xs[i][t]
		}
		rows = append(rows, row)
		ys = append(ys, y[t])
	}
	beta, r2, err := OLS(rows, ys)
	if err != nil {
		return Attribution{}, err
	}

	a := Attribution{Date: dates[len(dates)-1], Total: y[last], R2: r2, Window: len(ys)}
	explained := 0.0
	for i, f := range factors {
		c := Contribution{Name: f.Name, Beta: beta[i+1], Move: xs[i][last], Kind: "return"}
		if f.Kind == FactorDiffBP {
			c.Kind = "bp"
		}
		c.Contribution = c.Beta * c.Move
		explained += c.Contribution
		a.Contributions = append(a.Contributions, c)
	}
	a.Residual = a.Total - explained

	sum := math.Abs(a.Residual)
	for _, c := range a.Contributions {
		sum += math.Abs(c.Contribution)
	}
	if sum > 0 {
		for i := range a.Contributions {
			a.Contributions[i].Share = math.Abs(a.Contributions[i].Contribution) / sum
		}
		a.ResidualShare = math.Abs(a.Residual) / sum
	}
	return a, nil
}

// OLS 解正规方程 (XᵀX)β = Xᵀy，返回系数和 R²。
func OLS(x [][]float64, y []float64) ([]float64, float64, error) {
	if len(x) == 0 || len(x) != len(y) {
		return nil, 0, errors.New("ols: 样本为空")
	}
	k := len(x[0])
	// 增广矩阵 [XᵀX | Xᵀy]
	m := make([][]float64, k)
	for i := range m {
		m[i] = make([]float64, k+1)
	}
	for t, row := range x {
		for i := range k {
			for j := range k {
				m[i][j] += row[i] * row[j]
			}
			m[i][k] += row[i] * y[t]
		}
	}
	// 带部分主元的高斯消元
	for c := range k {
		p := c
		for r := c + 1; r < k; r++ {
			if math.Abs(m[r][c]) > math.Abs(m[p][c]) {
				p = r
			}
		}
		if math.Abs(m[p][c]) < 1e-12 {
			return nil, 0, errors.New("ols: 因子共线，无法求解")
		}
		m[c], m[p] = m[p], m[c]
		for r := range k {
			if r == c {
				continue
			}
			f := m[r][c] / m[c][c]
			for j := c; j <= k; j++ {
				m[r][j] -= f * m[c][j]
			}
		}
	}
	beta := make([]float64, k)
	for i := range k {
		beta[i] = m[i][k] / m[i][i]
	}

	var mean float64
	for _, v := range y {
		mean += v
	}
	mean /= float64(len(y))
	var ssRes, ssTot float64
	for t, row := range x {
		var pred float64
		for i := range k {
			pred += row[i] * beta[i]
		}
		ssRes += (y[t] - pred) * (y[t] - pred)
		ssTot += (y[t] - mean) * (y[t] - mean)
	}
	r2 := 0.0
	if ssTot > 0 {
		r2 = 1 - ssRes/ssTot
	}
	return beta, r2, nil
}
