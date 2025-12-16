package main

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"sync"
	"time"

	"github.com/fatih/color"
	"golang.org/x/time/rate"
)

// Config contient les paramètres du crawler.
type Config struct {
	TargetURL    string
	MaxDepth     int
	OnlyInternal bool
	OnlyExternal bool
	OutputPath   string
}

// Crawler représente un crawler web.
type Crawler struct {
	Config  Config
	Client  *http.Client
	Visited map[string]bool
	mu      sync.Mutex
	Results []string
	wg      sync.WaitGroup
}

// rateLimitedTransport implémente http.RoundTripper avec rate limiting.
type rateLimitedTransport struct {
	limiter *rate.Limiter
	base    http.RoundTripper
}

// RoundTrip exécute une requête HTTP avec rate limiting.
func (t *rateLimitedTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if err := t.limiter.Wait(req.Context()); err != nil {
		return nil, err
	}
	return t.base.RoundTrip(req)
}

func New(cfg Config) *Crawler {
	limiter := rate.NewLimiter(rate.Every(100*time.Millisecond), 10)

	return &Crawler{
		Config: cfg,
		Client: &http.Client{
			Timeout: 10 * time.Second,
			Transport: &rateLimitedTransport{
				limiter: limiter,
				base: &http.Transport{
					TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
				},
			},
		},
		Visited: make(map[string]bool, 1000),
	}
}

// Start lance le crawler.
func (c *Crawler) Start() error {
	parsed, err := url.Parse(c.Config.TargetURL)
	if err != nil {
		return err
	}
	norm := parsed.String()
	c.mu.Lock()
	c.Visited[norm] = true
	c.mu.Unlock()

	if err := c.crawl(norm, 0); err != nil {
		return err
	}
	c.wg.Wait()
	return nil
}

// crawl explore récursivement les liens.
func (c *Crawler) crawl(rawURL string, depth int) error {
	if depth >= c.Config.MaxDepth {
		return nil
	}
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return err
	}

	resp, err := c.Client.Get(rawURL)
	if err != nil {
		fmt.Printf("[%s] %s: %v\n", color.RedString("ERR"), rawURL, err)
		return nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	for _, link := range Extract(string(body)) {
		res, err := parsed.Parse(link)
		if err != nil {
			continue
		}
		abs := res.String()

		c.mu.Lock()
		if c.Visited[abs] {
			c.mu.Unlock()
			continue
		}
		c.Visited[abs] = true

		if res.Host != parsed.Host {
			if !c.Config.OnlyInternal {
				fmt.Printf("[%s] %s\n", color.CyanString("EXT"), abs)
				c.Results = append(c.Results, abs)
			}
			c.mu.Unlock()
		} else {
			if !c.Config.OnlyExternal {
				fmt.Printf("[%s] %s\n", color.GreenString("INT"), abs)
				c.Results = append(c.Results, abs)
			}
			c.mu.Unlock()

			c.wg.Add(1)
			go func(url string, d int) {
				defer c.wg.Done()
				c.crawl(url, d+1)
			}(abs, depth)
		}
	}
	return nil
}

// SaveJSON sauvegarde les résultats en JSON.
func (c *Crawler) SaveJSON() error {
	if c.Config.OutputPath == "" {
		return nil
	}
	type Export struct {
		Target  string   `json:"target"`
		Results []string `json:"results"`
		Count   int      `json:"count"`
	}
	data := Export{
		Target:  c.Config.TargetURL,
		Results: c.Results,
		Count:   len(c.Results),
	}
	file, err := os.Create(c.Config.OutputPath)
	if err != nil {
		return err
	}
	defer file.Close()
	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	return encoder.Encode(data)
}
