// Package portfolio 保存每个用户自己的交易记录，并按移动加权平均成本法计算持仓和盈亏。
package portfolio

import (
	"cmp"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"
)

type Transaction struct {
	ID        string    `json:"id"`
	Symbol    string    `json:"symbol"`
	Side      string    `json:"side"` // "buy" | "sell"
	Quantity  float64   `json:"quantity"`
	Price     float64   `json:"price"`
	Fee       float64   `json:"fee"`
	Date      string    `json:"date"` // YYYY-MM-DD
	Note      string    `json:"note"`
	CreatedAt time.Time `json:"createdAt"`
}

// ValidationError 是用户输入有问题，接口层据此返回 400。
type ValidationError struct{ msg string }

func (e ValidationError) Error() string { return e.msg }

func invalid(format string, args ...any) error { return ValidationError{fmt.Sprintf(format, args...)} }

var symbolRe = regexp.MustCompile(`^[A-Z0-9^=.\-]{1,15}$`)

// Normalize 清洗并校验一条用户提交的交易。
func (t *Transaction) Normalize() error {
	t.Symbol = strings.ToUpper(strings.TrimSpace(t.Symbol))
	t.Note = strings.TrimSpace(t.Note)
	switch {
	case !symbolRe.MatchString(t.Symbol):
		return invalid("代码格式不正确，例如 VOO、GC=F、^GSPC")
	case t.Side != "buy" && t.Side != "sell":
		return invalid("方向只能是买入或卖出")
	case !(t.Quantity > 0) || math.IsInf(t.Quantity, 0):
		return invalid("数量必须大于 0")
	case !(t.Price > 0) || math.IsInf(t.Price, 0):
		return invalid("价格必须大于 0")
	case t.Fee < 0 || math.IsNaN(t.Fee) || math.IsInf(t.Fee, 0):
		return invalid("手续费不能为负")
	case len([]rune(t.Note)) > 200:
		return invalid("备注最多 200 字")
	}
	d, err := time.Parse(time.DateOnly, t.Date)
	if err != nil {
		return invalid("日期格式应为 YYYY-MM-DD")
	}
	if d.After(time.Now().Add(24 * time.Hour)) {
		return invalid("日期不能晚于今天")
	}
	return nil
}

type Holding struct {
	Symbol    string  `json:"symbol"`
	Quantity  float64 `json:"quantity"`
	AvgCost   float64 `json:"avgCost"`
	CostBasis float64 `json:"costBasis"`
	Realized  float64 `json:"realized"` // 已实现盈亏（扣除手续费）
}

const eps = 1e-9

// Holdings 按日期顺序重放交易。卖出数量超过当时持仓时返回错误。
// 清仓的品种也会返回（数量为 0），以便展示已实现盈亏。
func Holdings(txs []Transaction) ([]Holding, error) {
	sorted := slices.Clone(txs)
	slices.SortStableFunc(sorted, func(a, b Transaction) int {
		return cmp.Or(strings.Compare(a.Date, b.Date), a.CreatedAt.Compare(b.CreatedAt))
	})
	bySymbol := map[string]*Holding{}
	var order []string
	for _, t := range sorted {
		h, ok := bySymbol[t.Symbol]
		if !ok {
			h = &Holding{Symbol: t.Symbol}
			bySymbol[t.Symbol] = h
			order = append(order, t.Symbol)
		}
		switch t.Side {
		case "buy":
			h.CostBasis += t.Quantity*t.Price + t.Fee
			h.Quantity += t.Quantity
		case "sell":
			if t.Quantity > h.Quantity+eps {
				return nil, invalid("%s 在 %s 卖出 %g，超过当时持仓 %g", t.Symbol, t.Date, t.Quantity, h.Quantity)
			}
			avg := h.CostBasis / h.Quantity
			h.Realized += t.Quantity*(t.Price-avg) - t.Fee
			h.CostBasis -= avg * t.Quantity
			h.Quantity -= t.Quantity
			if h.Quantity < eps {
				h.Quantity, h.CostBasis = 0, 0
			}
		}
	}
	out := make([]Holding, 0, len(order))
	for _, sym := range order {
		h := *bySymbol[sym]
		if h.Quantity > 0 {
			h.AvgCost = h.CostBasis / h.Quantity
		}
		out = append(out, h)
	}
	return out, nil
}

// Store 把每个用户的交易存成 dataDir/portfolio/<用户 ID 的哈希>.json。
type Store struct {
	dir string
	mu  sync.Mutex
}

func NewStore(dataDir string) *Store { return &Store{dir: filepath.Join(dataDir, "portfolio")} }

func (s *Store) path(user string) string {
	h := sha256.Sum256([]byte(user))
	return filepath.Join(s.dir, hex.EncodeToString(h[:16])+".json")
}

func (s *Store) load(user string) ([]Transaction, error) {
	b, err := os.ReadFile(s.path(user))
	if errors.Is(err, os.ErrNotExist) {
		return []Transaction{}, nil
	}
	if err != nil {
		return nil, err
	}
	var txs []Transaction
	return txs, json.Unmarshal(b, &txs)
}

// save 先写临时文件再改名，进程中途退出也不会留下写了一半的文件
func (s *Store) save(user string, txs []Transaction) error {
	if err := os.MkdirAll(s.dir, 0o700); err != nil {
		return err
	}
	b, err := json.MarshalIndent(txs, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.path(user) + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, s.path(user))
}

func (s *Store) List(user string) ([]Transaction, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.load(user)
}

func (s *Store) Add(user string, t Transaction) (Transaction, error) {
	if err := t.Normalize(); err != nil {
		return Transaction{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	txs, err := s.load(user)
	if err != nil {
		return Transaction{}, err
	}
	if len(txs) >= 5000 {
		return Transaction{}, invalid("交易记录已达上限")
	}
	id := make([]byte, 8)
	rand.Read(id)
	t.ID, t.CreatedAt = hex.EncodeToString(id), time.Now()
	next := append(txs, t)
	if _, err := Holdings(next); err != nil {
		return Transaction{}, err
	}
	return t, s.save(user, next)
}

func (s *Store) Delete(user, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	txs, err := s.load(user)
	if err != nil {
		return err
	}
	i := slices.IndexFunc(txs, func(t Transaction) bool { return t.ID == id })
	if i < 0 {
		return os.ErrNotExist
	}
	next := slices.Delete(txs, i, i+1)
	// 删掉一笔买入可能让之后的卖出变得不合法
	if _, err := Holdings(next); err != nil {
		return invalid("删除后持仓会变成负数：%v", err)
	}
	return s.save(user, next)
}
