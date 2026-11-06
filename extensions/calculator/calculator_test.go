package calculator_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"sync/atomic"
	"testing"
	"time"

	"github.com/JoaoVitorOli/althea/extensions/calculator"
	"github.com/JoaoVitorOli/althea/sdk/sdktest"
)

func TestAnswersMathAndCopiesTheResult(t *testing.T) {
	ext := calculator.New()
	ctx := sdktest.New(t, ext, "pt-BR")
	item, err := ext.Answer(ctx, "1234,5 * 2")
	if err != nil || item == nil {
		t.Fatalf("Answer = %v, %v", item, err)
	}
	if item.Title != "2.469" || item.Subtitle != "1234,5 * 2" || item.Accessory != "Calculadora" {
		t.Errorf("item = %+v", item)
	}
	if err := item.Actions[0].Run(ctx); err != nil {
		t.Fatal(err)
	}
	if err := item.Actions[1].Run(ctx); err != nil {
		t.Fatal(err)
	}
	if want := []string{"2469", "1234,5 * 2 = 2.469"}; !slices.Equal(ctx.Copied, want) {
		t.Errorf("copied %q, want %q", ctx.Copied, want)
	}
	if ctx.Hidden != 2 || len(ctx.Toasts) == 0 {
		t.Errorf("hid %d times, toasts %v", ctx.Hidden, ctx.Toasts)
	}
}

func TestDoesNotAnswerWhatIsNotMath(t *testing.T) {
	ext := calculator.New()
	ctx := sdktest.New(t, ext, "en")
	for _, q := range []string{"chrome", "7", "windows 11", "2 +"} {
		if item, err := ext.Answer(ctx, q); item != nil || err != nil {
			t.Errorf("Answer(%q) = %+v, %v; want nothing", q, item, err)
		}
	}
}

// rateServer answers as open.er-api.com does, counting the requests, or fails while failing is
// set.
type rateServer struct {
	*httptest.Server
	hits    atomic.Int32
	failing atomic.Bool
}

func newRateServer(t *testing.T) *rateServer {
	s := &rateServer{}
	s.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		s.hits.Add(1)
		if s.failing.Load() {
			http.Error(w, "down", http.StatusServiceUnavailable)
			return
		}
		next := time.Now().Add(time.Hour).Unix()
		fmt.Fprintf(w, `{"result":"success","base_code":"USD","time_last_update_unix":%d,
			"time_next_update_unix":%d,"rates":{"USD":1,"BRL":5,"EUR":0.8}}`, time.Now().Unix(), next)
	}))
	t.Cleanup(s.Close)
	return s
}

func TestConvertsMoneyOnceTheRatesArrive(t *testing.T) {
	srv := newRateServer(t)
	ext := calculator.NewWithRates(srv.URL, time.Now)
	ctx := sdktest.New(t, ext, "pt-BR")
	ctx.Country = "BR"

	item, _ := ext.Answer(ctx, "50 dollars")
	if item == nil || item.Title != "Buscando cotações…" {
		t.Fatalf("first answer = %+v, want the rates on their way", item)
	}
	ctx.Wait()
	if ctx.Refreshed != 1 {
		t.Errorf("refreshed %d times, want once the rates arrived", ctx.Refreshed)
	}

	item, _ = ext.Answer(ctx, "50 dollars")
	if item == nil || item.Title != "R$ 250,00" || item.Subtitle != "1 USD = 5 BRL" {
		t.Fatalf("answer = %+v", item)
	}
	if item.Accessory != "Cotações da ExchangeRate-API" {
		t.Errorf("accessory = %q, want the credit the API asks for", item.Accessory)
	}
	for _, a := range item.Actions[:2] {
		if err := a.Run(ctx); err != nil {
			t.Fatal(err)
		}
	}
	if want := []string{"250,00", "R$ 250,00"}; !slices.Equal(ctx.Copied, want) {
		t.Errorf("copied %q, want %q", ctx.Copied, want)
	}

	// Kept, and not fetched again before the API's next update, even by a new instance.
	if item, _ := ext.Answer(ctx, "10 eur em reais"); item == nil || item.Title != "R$ 62,50" {
		t.Errorf("answer = %+v", item)
	}
	again := calculator.NewWithRates(srv.URL, time.Now)
	ctx2 := sdktest.New(t, again, "en")
	ctx2.Store = ctx.Store
	if item, _ := again.Answer(ctx2, "$10 to brl"); item == nil || item.Title != "R$ 50.00" {
		t.Errorf("answer from kept rates = %+v", item)
	}
	ctx.Wait()
	ctx2.Wait()
	if n := srv.hits.Load(); n != 1 {
		t.Errorf("fetched %d times, want 1", n)
	}
}

func TestFetchesAgainWhenTheRatesAreOld(t *testing.T) {
	srv := newRateServer(t)
	now := time.Now()
	ext := calculator.NewWithRates(srv.URL, func() time.Time { return now })
	ctx := sdktest.New(t, ext, "en")
	_, _ = ext.Answer(ctx, "1 usd")
	ctx.Wait()

	now = now.Add(25 * time.Hour)
	if item, _ := ext.Answer(ctx, "1 usd in brl"); item == nil || item.Title != "R$ 5.00" {
		t.Errorf("answer while fetching = %+v, want the old rates meanwhile", item)
	}
	ctx.Wait()
	if n := srv.hits.Load(); n != 2 {
		t.Errorf("fetched %d times, want 2", n)
	}
}

func TestSaysWhenTheRatesCannotBeFetched(t *testing.T) {
	srv := newRateServer(t)
	srv.failing.Store(true)
	ext := calculator.NewWithRates(srv.URL, time.Now)
	ctx := sdktest.New(t, ext, "en")
	_, _ = ext.Answer(ctx, "5 eur")
	ctx.Wait()

	item, _ := ext.Answer(ctx, "5 eur")
	if item == nil || item.Title != "Exchange rates could not be fetched" || len(item.Actions) != 1 {
		t.Fatalf("answer = %+v", item)
	}
	ctx.Wait()
	if n := srv.hits.Load(); n != 1 {
		t.Errorf("fetched %d times, want no retry within a minute", n)
	}

	srv.failing.Store(false)
	if err := item.Actions[0].Run(ctx); err != nil {
		t.Fatal(err)
	}
	ctx.Wait()
	if item, _ := ext.Answer(ctx, "5 eur"); item == nil || item.Title != "$6.25" {
		t.Errorf("answer after Try Again = %+v", item)
	}
}

func TestLeavesMoneyAloneWithoutRates(t *testing.T) {
	srv := newRateServer(t)
	ext := calculator.NewWithRates(srv.URL, time.Now)
	ctx := sdktest.New(t, ext, "en")
	ctx.Prefs["exchange_rates"] = "false"
	if item, _ := ext.Answer(ctx, "50 dollars"); item != nil {
		t.Errorf("answer = %+v, want none", item)
	}
	ctx.Wait()
	if n := srv.hits.Load(); n != 0 {
		t.Errorf("fetched %d times, want none", n)
	}
}

func TestDefaultsToTheRegionsCurrency(t *testing.T) {
	srv := newRateServer(t)
	ext := calculator.NewWithRates(srv.URL, time.Now)
	ctx := sdktest.New(t, ext, "en")
	ctx.Country = "PT"
	_, _ = ext.Answer(ctx, "1 usd")
	ctx.Wait()
	if item, _ := ext.Answer(ctx, "10 usd"); item == nil || item.Title != "€8.00" {
		t.Errorf("answer in Portugal = %+v", item)
	}
	ctx.Prefs["currency"] = "brl"
	if item, _ := ext.Answer(ctx, "10 usd"); item == nil || item.Title != "R$ 50.00" {
		t.Errorf("answer with BRL set = %+v", item)
	}
}
