package market

import (
	"testing"
	"time"
)

func TestTradingDate(t *testing.T) {
	at := func(s string) time.Time {
		v, err := time.ParseInLocation("2006-01-02 15:04", s, NewYork)
		if err != nil {
			t.Fatal(err)
		}
		return v
	}
	for in, want := range map[string]string{
		"2026-10-29 16:00": "2026-10-29", // 周四收盘
		"2026-10-29 19:00": "2026-10-30", // 周四夜盘属于周五
		"2026-10-30 19:00": "2026-11-02", // 周五 18:00 后算到下周一
		"2026-11-01 19:00": "2026-11-02", // 周日晚上开盘属于周一
		"2026-10-31 12:00": "2026-11-02", // 周六
	} {
		if got := TradingDate(at(in)); got != want {
			t.Errorf("TradingDate(%s) = %s, want %s", in, got, want)
		}
	}
}
