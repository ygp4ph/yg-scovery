package main

import (
	"bufio"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/fatih/color"
)

type Config struct {
	TargetURL       string
	MaxDepth        int
	IncludeExternal bool
	OutputPath      string
	Verbose         bool
	ShowTree        bool
}

type Crawler struct {
	cfg   Config
	cli   *http.Client
	vis   sync.Map
	val   sync.Map
	res   []string
	resMu sync.Mutex
	wg    sync.WaitGroup
	sem   chan struct{}
}

// New creates and initializes a new Crawler instance with the specified configuration.
func New(cfg Config) *Crawler {
	return &Crawler{
		cfg: cfg,
		cli: &http.Client{
			Timeout: 60 * time.Second,
			Transport: &http.Transport{
				TLSClientConfig:     &tls.Config{},
				MaxIdleConns:        100,
				MaxIdleConnsPerHost: 20,
				IdleConnTimeout:     30 * time.Second,
			},
		},
		sem: make(chan struct{}, max(16, runtime.NumCPU()*4)),
	}
}

// Start initiates the crawling process. It performs an initial connection check, scans for robots.txt and sitemap.xml, and begins recursive crawling.
func (c *Crawler) Start() error {
	base, err := url.Parse(c.cfg.TargetURL)
	if err != nil {
		return err
	}

	if err := c.checkConn(base.String()); err != nil {
		return err
	}
	c.vis.Store(base.String(), true)

	for _, path := range []string{"/robots.txt", "/sitemap.xml"} {
		u := base.ResolveReference(&url.URL{Path: path}).String()
		if _, loaded := c.vis.LoadOrStore(u, true); !loaded {
			if body, ok := c.fetch(u); ok {
				if path == "/robots.txt" {
					paths, sitemaps := ExtractRobots(body)
					c.enqueue(paths, base, 1)
					for _, sm := range sitemaps {
						if _, loaded := c.vis.LoadOrStore(sm, true); !loaded {
							if b, ok := c.fetch(sm); ok {
								c.enqueue(ExtractSitemap(b), base, 1)
							}
						}
					}
				} else {
					c.enqueue(ExtractSitemap(body), base, 1)
				}
			}
		}
	}

	c.wg.Add(1)
	go func() { defer c.wg.Done(); c.crawl(base.String(), 0) }()
	c.wg.Wait()
	return nil
}

// checkConn verifies if the specified URL is reachable using a HEAD or GET request.
func (c *Crawler) checkConn(u string) error {
	if err := c.req(u, "HEAD", 30*time.Second); err != nil && !strings.Contains(err.Error(), "aborted") {
		return c.req(u, "GET", 30*time.Second)
	} else if err != nil {
		return err
	}
	return nil
}

// req performs an HTTP request with the given method and timeout, returning an error if the request fails or returns an error status code.
// It can optionally skip TLS certificate verification if the user agrees.
func (c *Crawler) req(u, method string, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, method, u, nil)
	resp, err := c.cli.Do(req)

	if err != nil && (strings.Contains(err.Error(), "x509") || strings.Contains(err.Error(), "certificate")) {
		if c.cli.Transport.(*http.Transport).TLSClientConfig.InsecureSkipVerify {
			return err
		}
		fmt.Printf("%s Target certificate invalid. Proceed? [Y/n]: ", color.YellowString("[!]"))
		ans, _ := bufio.NewReader(os.Stdin).ReadString('\n')
		if a := strings.ToLower(strings.TrimSpace(ans)); a == "" || a == "y" || a == "yes" {
			c.cli.Transport.(*http.Transport).TLSClientConfig.InsecureSkipVerify = true
			color.Yellow("[WRN] SSL verification disabled")
			return c.req(u, method, timeout)
		}
		return fmt.Errorf("aborted by user")
	} else if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode == 404 {
		return fmt.Errorf("404 not found")
	}
	if resp.StatusCode >= 400 && resp.StatusCode != 405 {
		return fmt.Errorf("status %d", resp.StatusCode)
	}
	if resp.StatusCode == 405 {
		return fmt.Errorf("405 method not allowed")
	}
	return nil
}

// fetch retrieves the content of the given URL. Returns the response body and a boolean indicating success.
func (c *Crawler) fetch(u string) (string, bool) {
	resp, err := c.cli.Get(u)
	if err != nil || resp.StatusCode != 200 {
		return "", false
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return string(b), true
}

// crawl recursively fetches and processes a URL up to the max depth configured limit.
func (c *Crawler) crawl(u string, depth int) {
	if depth >= c.cfg.MaxDepth {
		return
	}
	base, _ := url.Parse(u)

	c.sem <- struct{}{}
	b, ok := c.fetch(u)
	<-c.sem

	if ok {
		c.enqueue(Extract(b), base, depth+1)
	}
}

// enqueue processes a list of found links, validates them concurrently, and adds valid ones to the crawl queue.
func (c *Crawler) enqueue(links []string, base *url.URL, nextDepth int) {
	var wg sync.WaitGroup
	var mu sync.Mutex
	type info struct {
		u   string
		ext bool
	}
	var valid []info

	for _, l := range links {
		wg.Add(1)
		go func(raw string) {
			defer wg.Done()
			c.sem <- struct{}{}
			defer func() { <-c.sem }()

			u, err := base.Parse(raw)
			if err != nil {
				return
			}
			abs, ext := u.String(), u.Host != base.Host
			if ext && !c.cfg.IncludeExternal {
				return
			}

			ok := false
			if v, found := c.val.Load(abs); found {
				ok = v.(bool)
			} else {
				ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
				defer cancel()
				req, _ := http.NewRequestWithContext(ctx, "HEAD", abs, nil)
				resp, err := c.cli.Do(req)
				ok = err == nil && resp.StatusCode < 400
				if resp != nil {
					resp.Body.Close()
				}
				if !ok && c.cfg.Verbose && err != nil {
					fmt.Printf("[%s] %s: %v\n", color.RedString("ERR"), abs, err)
				}
				c.val.Store(abs, ok)
			}
			if ok {
				mu.Lock()
				valid = append(valid, info{abs, ext})
				mu.Unlock()
			}
		}(l)
	}

	wg.Wait()

	for _, v := range valid {
		if _, loaded := c.vis.LoadOrStore(v.u, true); !loaded {
			prefix, col := "INT", color.GreenString
			if v.ext {
				prefix, col = "EXT", color.CyanString
			}

			fmt.Printf("[%s] %s\n", col(prefix), v.u)
			c.resMu.Lock()
			c.res = append(c.res, v.u)
			c.resMu.Unlock()

			if !v.ext {
				c.wg.Add(1)
				go func(u string) {
					defer c.wg.Done()
					c.crawl(u, nextDepth)
				}(v.u)
			}
		}
	}
}

type treeNode map[string]treeNode

// buildTree constructs a hierarchical tree representation of all validated URLs.
func (c *Crawler) buildTree() treeNode {
	root, base := treeNode{}, c.cfg.TargetURL
	uBase, _ := url.Parse(base)
	for _, raw := range append([]string{base}, c.res...) {
		u, _ := url.Parse(raw)
		if u == nil || u.Host != uBase.Host {
			continue
		}

		path := u.Path
		if path == "" {
			path = "/"
		}
		parts := strings.Split(path, "/")
		curr := root
		for i, p := range parts {
			if p == "" {
				continue
			}
			if i == len(parts)-1 && u.RawQuery != "" {
				p += "?" + u.RawQuery
			}
			if _, ok := curr[p]; !ok {
				curr[p] = treeNode{}
			}
			curr = curr[p]
		}
		if path == "/" && u.RawQuery != "" {
			if _, ok := root["?"+u.RawQuery]; !ok {
				root["?"+u.RawQuery] = treeNode{}
			}
		}
	}
	return root
}

// PrintTree outputs the URL hierarchy to the standard output in a tree structure.
func (c *Crawler) PrintTree() {
	if !c.cfg.ShowTree {
		return
	}
	fmt.Printf("\n%s\n%s\n", color.MagentaString("=== Site Tree ==="), c.cfg.TargetURL)
	var printNode func(treeNode, string)
	printNode = func(n treeNode, pfx string) {
		keys := make([]string, 0, len(n))
		for k := range n {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for i, k := range keys {
			conn, npfx := "├── ", pfx+"│   "
			if i == len(keys)-1 {
				conn, npfx = "└── ", pfx+"    "
			}
			fmt.Printf("%s%s%s\n", pfx, conn, k)
			printNode(n[k], npfx)
		}
	}
	printNode(c.buildTree(), "")
}

// SaveJSON exports the crawling results, including the URL tree if enabled, into a JSON file format.
func (c *Crawler) SaveJSON() error {
	var t treeNode
	if c.cfg.ShowTree {
		t = c.buildTree()
	}
	f, err := os.Create(c.cfg.OutputPath)
	if err != nil {
		return err
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	return enc.Encode(map[string]any{"target": c.cfg.TargetURL, "results": c.res, "tree": t, "count": len(c.res)})
}
