package main

import (
	"context"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"io"
	"log"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// rows reads pages of 100 rows from a Hugging Face dataset
// (datasets-server). pages = 0: the whole dataset; otherwise pages
// drawn at random, based on seed. Pages are kept in dir/pages, so that
// a run stopped by the rate limit resumes where it stopped.
func rows(ctx context.Context, dir, name, split string, total, pages int, seed string, fn func(map[string]any)) error {
	n := (total + 99) / 100
	offsets := make([]int, 0, n)
	if pages == 0 || pages >= n {
		for p := 0; p < n; p++ {
			offsets = append(offsets, p*100)
		}
	} else {
		h := fnv.New64a()
		fmt.Fprint(h, name, seed)
		for _, p := range rand.New(rand.NewSource(int64(h.Sum64()))).Perm(n)[:pages] {
			offsets = append(offsets, p*100)
		}
	}
	cache := filepath.Join(dir, "pages", strings.ReplaceAll(name, "/", "_"))
	if err := os.MkdirAll(cache, 0o755); err != nil {
		return err
	}
	for _, off := range offsets {
		path := filepath.Join(cache, fmt.Sprintf("%s-%d.json", split, off))
		body, err := os.ReadFile(path)
		if err != nil {
			u := fmt.Sprintf("https://datasets-server.huggingface.co/rows?dataset=%s&config=default&split=%s&offset=%d&length=100",
				url.QueryEscape(name), split, off)
			if body, err = get(ctx, u); err != nil {
				return err
			}
			if err := os.WriteFile(path, body, 0o644); err != nil {
				return err
			}
			time.Sleep(time.Second) // goes easy on the datasets-server
		}
		var page struct {
			Rows []struct {
				Row map[string]any `json:"row"`
			} `json:"rows"`
		}
		if err := json.Unmarshal(body, &page); err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
		for _, r := range page.Rows {
			fn(r.Row)
		}
	}
	return nil
}

// get reads u, waiting out rate limits and server errors. HF_TOKEN, when
// set, raises the rate limit.
func get(ctx context.Context, u string) ([]byte, error) {
	delay := 5 * time.Second
	for attempt := 1; ; attempt++ {
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
		if token := os.Getenv("HF_TOKEN"); token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			return nil, err
		}
		if res.StatusCode == http.StatusOK {
			body, err := io.ReadAll(res.Body)
			res.Body.Close()
			return body, err
		}
		res.Body.Close()
		if (res.StatusCode != http.StatusTooManyRequests && res.StatusCode < 500) || attempt == 10 {
			return nil, fmt.Errorf("%s: HTTP %d", u, res.StatusCode)
		}
		wait := delay
		if s, err := strconv.Atoi(res.Header.Get("Retry-After")); err == nil && s > 0 {
			wait = time.Duration(s) * time.Second
		}
		log.Printf("HTTP %d, retrying in %s", res.StatusCode, wait)
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(wait):
		}
		delay = min(2*delay, 5*time.Minute)
	}
}
