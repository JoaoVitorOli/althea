package calculator

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/JoaoVitorOli/althea/extensions/calculator/calc"
	"github.com/JoaoVitorOli/althea/sdk"
)

// ratesURL is ExchangeRate-API's open access: daily rates of about 160 currencies, no key, on the
// condition that answers credit it (https://www.exchangerate-api.com/docs/free).
const ratesURL = "https://open.er-api.com/v6/latest/USD"

// ratesKey is where the rates are kept in the extension's Storage, so that a restart, or a day
// offline, still converts.
const ratesKey = "rates"

const (
	// fetchTimeout bounds one fetch.
	fetchTimeout = 15 * time.Second
	// retryAfter is how long a failed fetch waits for the next try.
	retryAfter = time.Minute
	// maxAge is when rates are fetched again even if the API said they would be later.
	maxAge = 24 * time.Hour
)

// stored are rates as Storage keeps them.
type stored struct {
	Rates calc.Rates `json:"rates"`
	// Updated is when the API updated them, Next when it will again.
	Updated time.Time `json:"updated"`
	Next    time.Time `json:"next"`
}

// rates holds the exchange rates, fetched once a day at most, and only when a calculation needs
// them.
type rates struct {
	url string
	now func() time.Time

	mu       sync.Mutex
	loaded   bool
	cur      stored
	fetching bool
	failed   time.Time
}

// get returns the rates at hand, nil while there are none, and starts fetching new ones in the
// background when they are due: Refresh asks the search again once they arrive.
func (r *rates) get(ctx sdk.Context) (calc.Rates, time.Time) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.loaded {
		r.loaded = true
		if b, ok, err := ctx.Storage().Get(ctx, ratesKey); err != nil {
			ctx.Log().Warn("exchange rates not read", "err", err)
		} else if ok {
			if err := json.Unmarshal(b, &r.cur); err != nil {
				ctx.Log().Warn("exchange rates not read", "err", err)
			}
		}
	}
	now := r.now()
	due := r.cur.Rates == nil || now.After(r.cur.Next) || now.Sub(r.cur.Updated) > maxAge
	if due && !r.fetching && now.Sub(r.failed) >= retryAfter {
		r.fetching = true
		ctx.Go(r.fetch)
	}
	return r.cur.Rates, r.cur.Updated
}

// pending reports whether rates are on their way, and none at hand.
func (r *rates) pending() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.cur.Rates == nil && r.fetching
}

// retry forgets the last failure, for a fetch the user asked for.
func (r *rates) retry() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.failed = time.Time{}
}

func (r *rates) fetch(ctx sdk.Context) {
	got, err := r.download(ctx)
	r.mu.Lock()
	r.fetching = false
	if err != nil {
		r.failed = r.now()
		r.mu.Unlock()
		ctx.Log().Warn("exchange rates not fetched", "err", err)
		ctx.Refresh() // to say so, where the answer waits
		return
	}
	r.cur, r.failed = got, time.Time{}
	r.mu.Unlock()
	if b, err := json.Marshal(got); err == nil {
		if err := ctx.Storage().Set(ctx, ratesKey, b); err != nil {
			ctx.Log().Warn("exchange rates not kept", "err", err)
		}
	}
	ctx.Refresh()
}

func (r *rates) download(ctx sdk.Context) (stored, error) {
	c, cancel := context.WithTimeout(ctx, fetchTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(c, http.MethodGet, r.url, http.NoBody)
	if err != nil {
		return stored{}, err
	}
	res, err := ctx.HTTP().Do(req)
	if err != nil {
		return stored{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return stored{}, fmt.Errorf("exchange rates: %s", res.Status)
	}
	var body struct {
		Result  string     `json:"result"`
		Error   string     `json:"error-type"`
		Updated int64      `json:"time_last_update_unix"`
		Next    int64      `json:"time_next_update_unix"`
		Base    string     `json:"base_code"`
		Rates   calc.Rates `json:"rates"`
	}
	if err := json.NewDecoder(io.LimitReader(res.Body, 1<<20)).Decode(&body); err != nil {
		return stored{}, fmt.Errorf("exchange rates: %w", err)
	}
	if body.Result != "success" || body.Base != "USD" || body.Rates["USD"] != 1 {
		return stored{}, fmt.Errorf("exchange rates: %s %s", body.Result, body.Error)
	}
	return stored{Rates: body.Rates, Updated: time.Unix(body.Updated, 0), Next: time.Unix(body.Next, 0)}, nil
}
