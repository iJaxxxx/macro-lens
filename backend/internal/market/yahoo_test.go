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
