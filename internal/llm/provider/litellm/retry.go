package litellm

import (
	"context"
	"errors"
	"math/rand"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Mpaape/AurumCode/internal/llm"
	"github.com/Mpaape/AurumCode/internal/llm/tokens"
)

// A plain rate limit (429 without the text that says this request alone is
// larger than the limit, llm.DetectPromptTooLong) is transient: the provider
// is waited out and the same request sent again. The wait is the one the
// provider asks for (retry-after-ms, then retry-after in seconds or as a
// date), or else an exponential backoff with jitter. The attempts end when
// the next wait would pass the provider timeout the review already has
// (llm.ProviderTimeout, counted from the first attempt), and every attempt
// has only what is left of that time; no size or count is capped.

// firstBackoff is the backoff of the first retry when the provider names
// no wait; each next retry doubles it.
const firstBackoff = time.Second

// postRetrying sends jsonData to url until it is answered with 200, a
// non-transient error, or the provider timeout would be passed.
func (p *Provider) postRetrying(url string, jsonData []byte) ([]byte, error) {
	deadline := time.Now().Add(llm.ProviderTimeout())
	ctx, cancel := context.WithDeadline(context.Background(), deadline)
	defer cancel()
	for attempt := 0; ; attempt++ {
		status, header, body, err := p.send(ctx, url, jsonData)
		if err != nil {
			return nil, err
		}
		if status == http.StatusOK {
			return body, nil
		}
		apiErr := confirmSize(p.apiError(status, body), status, jsonData)
		wait, retry := retryWait(status, header, apiErr, attempt)
		if !retry || time.Now().Add(wait).After(deadline) {
			return nil, apiErr
		}
		p.sleep(wait)
	}
}

// confirmSize keeps a 429 as a refusal for length only when this request
// alone does not fit the limit it names. Anthropic says "this request would
// exceed your rate limit of N input tokens per minute" both when the request
// is larger than N and when it is only larger than what is left of the
// minute; the request's own size, by the product's estimate, tells the two
// apart. The second is a plain rate limit: waited out and retried.
func confirmSize(err error, status int, jsonData []byte) error {
	var apiErr *llm.APIError
	if status != http.StatusTooManyRequests || !errors.As(err, &apiErr) || apiErr.TooLong == nil {
		return err
	}
	requested := tokens.Estimate(string(jsonData))
	if requested < apiErr.TooLong.Limit {
		apiErr.TooLong = nil
		return apiErr
	}
	apiErr.TooLong.Requested = requested
	return apiErr
}

// retryWait says whether an answer is a plain rate limit worth retrying, and
// how long to wait first.
func retryWait(status int, header http.Header, err error, attempt int) (time.Duration, bool) {
	if status != http.StatusTooManyRequests {
		return 0, false
	}
	var apiErr *llm.APIError
	if errors.As(err, &apiErr) && apiErr.TooLong != nil {
		return 0, false
	}
	if wait, ok := askedWait(header, time.Now()); ok {
		return wait, true
	}
	backoff := firstBackoff << attempt
	if backoff <= 0 {
		backoff = firstBackoff
	}
	return backoff/2 + time.Duration(rand.Int63n(int64(backoff))), true
}

// askedWait reads the wait the provider asked for: retry-after-ms, or
// retry-after as seconds or as an HTTP date.
func askedWait(header http.Header, now time.Time) (time.Duration, bool) {
	if raw := strings.TrimSpace(header.Get("Retry-After-Ms")); raw != "" {
		if ms, err := strconv.ParseFloat(raw, 64); err == nil && ms >= 0 {
			return time.Duration(ms * float64(time.Millisecond)), true
		}
	}
	raw := strings.TrimSpace(header.Get("Retry-After"))
	if raw == "" {
		return 0, false
	}
	if secs, err := strconv.ParseFloat(raw, 64); err == nil && secs >= 0 {
		return time.Duration(secs * float64(time.Second)), true
	}
	if at, err := http.ParseTime(raw); err == nil {
		if wait := at.Sub(now); wait > 0 {
			return wait, true
		}
		return 0, true
	}
	return 0, false
}
