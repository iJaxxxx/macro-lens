package portfolio

import (
	"errors"
	"math"
	"testing"
)

func tx(side, date string, qty, price, fee float64) Transaction {
	return Transaction{Symbol: "voo", Side: side, Date: date, Quantity: qty, Price: price, Fee: fee}
}

func TestHoldingsAverageCost(t *testing.T) {
	hs, err := Holdings([]Transaction{
		{Symbol: "VOO", Side: "sell", Date: "2026-03-01", Quantity: 5, Price: 600, Fee: 1},
		{Symbol: "VOO", Side: "buy", Date: "2026-01-01", Quantity: 10, Price: 500, Fee: 0},
		{Symbol: "VOO", Side: "buy", Date: "2026-02-01", Quantity: 10, Price: 520, Fee: 2},
	})
	if err != nil {
		t.Fatal(err)
	}
	h := hs[0]
	// 买入均价 (5000 + 5202) / 20 = 510.1；卖 5 股实现 5 × (600 - 510.1) - 1
	if h.Quantity != 15 || math.Abs(h.AvgCost-510.1) > 1e-9 || math.Abs(h.Realized-(5*89.9-1)) > 1e-9 {
		t.Errorf("holding = %+v", h)
	}
}

func TestHoldingsOversell(t *testing.T) {
	_, err := Holdings([]Transaction{{Symbol: "VOO", Side: "sell", Date: "2026-01-01", Quantity: 1, Price: 1}})
	var ve ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("err = %v, want ValidationError", err)
	}
}

func TestStore(t *testing.T) {
	s := NewStore(t.TempDir())
	buy, err := s.Add("alice", tx("buy", "2026-01-02", 3, 500, 0))
	if err != nil {
		t.Fatal(err)
	}
	if buy.Symbol != "VOO" || buy.ID == "" {
		t.Errorf("normalized = %+v", buy)
	}
	if _, err := s.Add("alice", tx("sell", "2026-01-03", 2, 510, 0)); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Add("alice", tx("sell", "2026-01-04", 5, 510, 0)); err == nil {
		t.Error("oversell accepted")
	}
	if _, err := s.Add("alice", tx("hold", "2026-01-04", 1, 1, 0)); err == nil {
		t.Error("bad side accepted")
	}
	// 空仓时的极小卖出不能产生 0/0
	if _, err := s.Add("carol", Transaction{Symbol: "VOO", Side: "sell", Date: "2026-01-04", Quantity: 1e-10, Price: 1}); err == nil {
		t.Error("tiny sell on empty position accepted")
	}
	// 超大的数量和价格会让 数量 × 价格 溢出
	if _, err := s.Add("carol", Transaction{Symbol: "VOO", Side: "buy", Date: "2026-01-04", Quantity: 1e200, Price: 1e200}); err == nil {
		t.Error("overflowing amount accepted")
	}
	// 删掉买入会让卖出不合法
	if err := s.Delete("alice", buy.ID); err == nil {
		t.Error("delete that breaks history accepted")
	}

	// 用户之间数据隔离
	if txs, _ := s.List("bob"); len(txs) != 0 {
		t.Errorf("bob sees %d transactions", len(txs))
	}
	if txs, _ := s.List("alice"); len(txs) != 2 {
		t.Errorf("alice has %d transactions", len(txs))
	}
}
