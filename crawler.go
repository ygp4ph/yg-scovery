package main

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"runtime"
	"sync"
	"time"

	"github.com/fatih/color"
)

// Config contient les paramètres du crawler.
type Config struct {
	TargetURL    string
	MaxDepth     int
	OnlyInternal bool
	OnlyExternal bool
	OutputPath   string
	Verbose      bool
}

// Crawler représente un crawler web optimisé.
type Crawler struct {
	Config     Config
	Client     *http.Client
	FastClient *http.Client // Client rapide pour HEAD requests
	Visited    sync.Map     // Map concurrente pour éviter les locks
	Results    []string
	resultsMu  sync.Mutex
	wg         sync.WaitGroup
	validCache sync.Map // Cache de validation des liens
	semaphore  chan struct{}
}

func New(cfg Config) *Crawler {
	workers := runtime.NumCPU() * 4
	if workers < 16 {
		workers = 16
	}

	// Transport optimisé avec pool de connexions
	transport := &http.Transport{
		TLSClientConfig:     &tls.Config{InsecureSkipVerify: true},
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 10,
		MaxConnsPerHost:     20,
		IdleConnTimeout:     30 * time.Second,
		DisableKeepAlives:   false,
	}

	return &Crawler{
		Config: cfg,
		Client: &http.Client{
			Timeout:   8 * time.Second,
			Transport: transport,
		},
		FastClient: &http.Client{
			Timeout:   3 * time.Second, // Timeout court pour HEAD
			Transport: transport,
		},
		semaphore: make(chan struct{}, workers),
	}
}

// Start lance le crawler.
func (c *Crawler) Start() error {
	parsed, err := url.Parse(c.Config.TargetURL)
	if err != nil {
		return err
	}
	norm := parsed.String()
	c.Visited.Store(norm, true)

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
		if c.Config.Verbose {
			fmt.Printf("[%s] %s: %v\n", color.RedString("ERR"), rawURL, err)
		}
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

	links := Extract(string(body))
	validLinks := c.validateLinksParallel(links, parsed)

	for _, linkInfo := range validLinks {
		abs := linkInfo.url
		isExternal := linkInfo.isExternal

		if _, loaded := c.Visited.LoadOrStore(abs, true); loaded {
			continue
		}

		if isExternal {
			if !c.Config.OnlyInternal {
				fmt.Printf("[%s] %s\n", color.CyanString("EXT"), abs)
				c.addResult(abs)
			}
		} else {
			if !c.Config.OnlyExternal {
				fmt.Printf("[%s] %s\n", color.GreenString("INT"), abs)
				c.addResult(abs)
			}

			c.wg.Add(1)
			go func(url string, d int) {
				defer c.wg.Done()
				c.semaphore <- struct{}{}
				defer func() { <-c.semaphore }()
				c.crawl(url, d+1)
			}(abs, depth)
		}
	}
	return nil
}

type linkInfo struct {
	url        string
	isExternal bool
}

// validateLinksParallel valide plusieurs liens en parallèle.
func (c *Crawler) validateLinksParallel(links []string, baseURL *url.URL) []linkInfo {
	results := make(chan linkInfo, len(links))
	var wg sync.WaitGroup

	for _, link := range links {
		wg.Add(1)
		go func(l string) {
			defer wg.Done()
			c.semaphore <- struct{}{}
			defer func() { <-c.semaphore }()

			res, err := baseURL.Parse(l)
			if err != nil {
				return
			}
			abs := res.String()
			isExternal := res.Host != baseURL.Host

			// Optimization: Filtre avant validation réseau
			if c.Config.OnlyInternal && isExternal {
				return
			}
			// Fix: Ne pas filtrer les liens internes ici si OnlyExternal est actif,
			// car on a besoin de les parcourir pour trouver des liens externes profonds.

			if c.validateLink(abs) {
				results <- linkInfo{
					url:        abs,
					isExternal: isExternal,
				}
			}
		}(link)
	}

	go func() {
		wg.Wait()
		close(results)
	}()

	var validated []linkInfo
	for li := range results {
		validated = append(validated, li)
	}
	return validated
}

// validateLink vérifie qu'un lien est valide (avec cache).
func (c *Crawler) validateLink(u string) bool {
	// Vérifier le cache
	if cached, ok := c.validCache.Load(u); ok {
		return cached.(bool)
	}

	req, err := http.NewRequest("HEAD", u, nil)
	if err != nil {
		c.validCache.Store(u, false)
		return false
	}

	resp, err := c.FastClient.Do(req)
	if err != nil {
		if c.Config.Verbose {
			fmt.Printf("[%s] %s: %v\n", color.RedString("ERR"), u, err)
		}
		c.validCache.Store(u, false)
		return false
	}
	defer resp.Body.Close()

	valid := resp.StatusCode >= 200 && resp.StatusCode < 400
	c.validCache.Store(u, valid)
	return valid
}

func (c *Crawler) addResult(url string) {
	c.resultsMu.Lock()
	c.Results = append(c.Results, url)
	c.resultsMu.Unlock()
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
