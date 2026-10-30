package market

import (
	"context"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"
)

func TestYahooQuote(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v8/finance/chart/GC=F" {
			t.Errorf("path = %q", r.URL.Path)
		}
		// 第一个点是前一天的，超出 20 小时窗口；第三个点是 null
		w.Write([]byte(`{"chart":{"result":[{
			"meta":{"regularMarketPrice":4007.9,"previousClose":4060.4},
			"timestamp":[1000,100000,100900,101800],
			"indicators":{"quote":[{"close":[1.0,4050.0,null,4007.9]}]}
		}],"error":null}}`))
	}))
	defer srv.Close()

	q, err := (&Yahoo{BaseURL: srv.URL, HTTP: srv.Client()}).Quote(context.Background(), "GC=F")
	if err != nil {
		t.Fatal(err)
	}
	if q.Price != 4007.9 || q.PrevClose != 4060.4 {
		t.Errorf("price = %v, prevClose = %v", q.Price, q.PrevClose)
	}
	if want := []float64{4050, 4007.9}; !slices.Equal(q.Points, want) {
		t.Errorf("points = %v, want %v", q.Points, want)
	}
}

func TestYahooError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()

	if _, err := (&Yahoo{BaseURL: srv.URL, HTTP: srv.Client()}).Quote(context.Background(), "^GSPC"); err == nil {
		t.Fatal("want error on 429")
	}
}

func TestYahooHistory(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("interval") != "1d" {
			t.Errorf("query = %q", r.URL.RawQuery)
		}
		// 纽约时间 10-29 09:30、10-30 09:30（null，跳过）、12:00、15:00；同一天保留最后一个
		w.Write([]byte(`{"chart":{"result":[{
			"meta":{"regularMarketPrice":3},
			"timestamp":[1761744600,1761831000,1761840000,1761850800],
			"indicators":{"quote":[{"close":[1.0,null,2.0,3.0]}]}
		}],"error":null}}`))
	}))
	defer srv.Close()

	bars, err := (&Yahoo{BaseURL: srv.URL, HTTP: srv.Client()}).History(context.Background(), "VOO")
	if err != nil {
		t.Fatal(err)
	}
	want := []Bar{{"2025-10-29", 1}, {"2025-10-30", 3}}
	if !slices.Equal(bars, want) {
		t.Errorf("bars = %v, want %v", bars, want)
	}
}
