package utils

import (
	"fmt"
	"io"
	"log"
	"math/rand/v2"
	"net/http"
	"strconv"
	"time"
)

// Retry policy for outbound requests (MEC platform and oneM2M CSE).
//
// Many MEC IPE instances in a cluster typically start at the same moment and hit
// the same MEC platform / CSE. Retrying on a fixed schedule would keep them all
// firing in lockstep, so the delay between attempts uses exponential backoff with
// "full jitter": each retry sleeps a random duration in [0, min(retryMaxDelay,
// retryBaseDelay*2^attempt)]. math/rand/v2 is seeded per process, so every
// instance draws a different delay and the retries spread out over time.
const (
	retryMaxAttempts = 5                      // total attempts, including the first one
	retryBaseDelay   = 500 * time.Millisecond // backoff ceiling for the first retry
	retryMaxDelay    = 8 * time.Second        // upper bound of the backoff ceiling
	retryMaxElapsed  = 45 * time.Second       // no new attempt is started after this
)

// doWithRetry sends the request produced by newReq, retrying on transport errors
// (timeouts, connection refused/reset, DNS failures, ...) and on responses that
// signal a transient server condition (408, 429, 5xx). Other responses, including
// 4xx client errors, are returned to the caller as-is.
//
// newReq is called once per attempt so that the request body can be replayed.
func doWithRetry(client *http.Client, newReq func() (*http.Request, error)) (*http.Response, error) {
	start := time.Now()

	for attempt := 0; ; attempt++ {
		req, err := newReq()
		if err != nil {
			return nil, fmt.Errorf("failed to create request: %v", err)
		}

		resp, err := client.Do(req)
		retryable, reason := shouldRetry(resp, err)
		if !retryable {
			return resp, err
		}

		delay := backoffDelay(attempt, resp)
		if attempt+1 >= retryMaxAttempts || time.Since(start)+delay > retryMaxElapsed {
			log.Printf("%s %s: giving up after %d attempt(s): %s", req.Method, req.URL, attempt+1, reason)
			return resp, err
		}

		// Release the connection of the response we are discarding
		if resp != nil {
			_, _ = io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
		}

		log.Printf("%s %s: attempt %d/%d failed (%s), retrying in %v",
			req.Method, req.URL, attempt+1, retryMaxAttempts, reason, delay.Round(time.Millisecond))
		time.Sleep(delay)
	}
}

// shouldRetry reports whether a request outcome is transient and worth retrying.
func shouldRetry(resp *http.Response, err error) (bool, string) {
	if err != nil {
		return true, err.Error()
	}
	switch {
	case resp.StatusCode == http.StatusRequestTimeout,
		resp.StatusCode == http.StatusTooManyRequests,
		resp.StatusCode >= 500:
		return true, resp.Status
	}
	return false, ""
}

// backoffDelay returns a full-jitter exponential backoff delay for the given
// attempt. If the server sent a Retry-After header (in seconds), it is used as a
// lower bound and jitter is added on top so instances still do not resync.
func backoffDelay(attempt int, resp *http.Response) time.Duration {
	ceiling := retryBaseDelay << attempt
	if ceiling <= 0 || ceiling > retryMaxDelay {
		ceiling = retryMaxDelay
	}
	delay := rand.N(ceiling + 1)

	if resp != nil {
		if secs, err := strconv.Atoi(resp.Header.Get("Retry-After")); err == nil && secs > 0 {
			delay += time.Duration(secs) * time.Second
		}
	}
	return delay
}
