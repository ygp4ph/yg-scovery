package main

import (
	"flag"
	"fmt"
	"log"
	"net/url"
	"os"
	"strings"

	"github.com/fatih/color"
)

const Version = "v2.3.1"

// main is the entry point of the GolDigger application, parsing CLI flags and starting the crawler.
func main() {
	var c Config
	var h, version bool

	flag.IntVar(&c.MaxDepth, "d", 3, "")
	flag.IntVar(&c.MaxDepth, "depth", 3, "")
	flag.BoolVar(&c.IncludeExternal, "e", false, "")
	flag.BoolVar(&c.IncludeExternal, "ext", false, "")
	flag.StringVar(&c.OutputPath, "o", "", "")
	flag.StringVar(&c.OutputPath, "output", "", "")
	flag.BoolVar(&c.ShowTree, "t", false, "")
	flag.BoolVar(&c.ShowTree, "tree", false, "")
	flag.BoolVar(&c.Verbose, "v", false, "")
	flag.BoolVar(&c.Verbose, "verbose", false, "")
	flag.BoolVar(&h, "h", false, "")
	flag.BoolVar(&h, "help", false, "")
	flag.BoolVar(&version, "version", false, "")

	banner := func() {
		color.Cyan(`
   ______      ______  _                      
  / ____/___  / / __ \(_)___ _____ ____  _____
 / / __/ __ \/ / / / / / __ `+"`"+`/ __ `+"`"+`/ _ \/ ___/
/ /_/ / /_/ / / /_/ / / /_/ / /_/ /  __/ /    
\____/\____/_/_____/_/\__, /\__, /\___/_/     
                     /____//____/             %s
`, Version)
	}

	flag.Usage = func() {
		banner()
		fmt.Fprintf(os.Stderr, "\nUSAGE: %s [flags] <url>\n\nFLAGS:\n  -d, --depth\tMax recursion (default 3)\n  -e, --ext\tInclude external links\n  -t, --tree\tShow internal links tree\n  -o, --output\tOutput file (JSON)\n  -v, --verbose\tShow errors\n  --version\tShow version\n  -h, --help\tShow help\n", os.Args[0])
	}

	// Separate flags and the positional URL argument so user can type `goldigger url -t`
	var args []string
	for _, arg := range os.Args[1:] {
		if !strings.HasPrefix(arg, "-") && c.TargetURL == "" {
			c.TargetURL = arg
		} else {
			args = append(args, arg)
		}
	}

	flag.CommandLine.Parse(args)

	if h {
		flag.Usage()
		return
	}
	if version {
		fmt.Printf("goldigger %s\n", Version)
		return
	}

	banner()
	if c.TargetURL == "" {
		color.Red("[ERR] <url> is required\nUse -h for help")
		os.Exit(1)
	}
	if !strings.HasPrefix(c.TargetURL, "http") {
		c.TargetURL = "https://" + c.TargetURL
	}
	if _, err := url.Parse(c.TargetURL); err != nil {
		color.Red("[ERR] Invalid URL: %v", err)
		os.Exit(1)
	}

	color.Green("[INF] Scanning %s (Depth: %d)", c.TargetURL, c.MaxDepth)
	if c.IncludeExternal {
		color.Yellow("[INF] Included: External links")
	}
	if c.ShowTree {
		color.Magenta("[INF] Tree view enabled (Internal links)")
	}
	if c.OutputPath != "" {
		color.Blue("[INF] Output will be saved to %s", c.OutputPath)
	}

	crawler := New(c)
	if err := crawler.Start(); err != nil {
		log.Fatalf("%s %v", color.RedString("[FATAL] Crawler failed:"), err)
	}

	crawler.PrintTree()
	if c.OutputPath != "" {
		if err := crawler.SaveJSON(); err != nil {
			color.Red("[ERR] Failed to save output: %v", err)
		} else {
			color.Green("[INF] Saved results to %s", c.OutputPath)
		}
	}
}
