package main

import (
	"regexp"
	"strings"
)

var (
	urlRe  = regexp.MustCompile(`(https?://[a-zA-Z0-9\-\.]+\.[a-zA-Z]{2,}(?:/[^"'\s<>` + "`" + `]*)?)`)
	pathRe = regexp.MustCompile(`["'](\.?\.?/[^"'\s<>` + "`" + `]+)["']`)
	attrRe = regexp.MustCompile(`(?:href|src)=["']([^"']+)["']`)
	robRe  = regexp.MustCompile(`(?i)(?:allow|disallow):\s*(/[^\s]*)`)
	rmapRe = regexp.MustCompile(`(?i)sitemap:\s*(https?://[^\s]*)`)
	smapRe = regexp.MustCompile(`(?i)<loc>\s*([^<\s]+)\s*</loc>`)
)

// extract is a helper function that uses a regular expression to find and return unique matches from a given string.
func extract(c string, re *regexp.Regexp, group int) []string {
	var res []string
	seen := make(map[string]bool)
	for _, m := range re.FindAllStringSubmatch(c, -1) {
		if len(m) > group {
			if s := strings.TrimSpace(m[group]); len(s) > 1 && !strings.ContainsAny(s, "\n ") && !seen[s] {
				seen[s], res = true, append(res, s)
			}
		}
	}
	return res
}

// Extract parses the provided content string and returns a slice of unique URLs found.
// It uses regular expressions to identify full URLs, absolute paths, and relative paths in attributes.
func Extract(c string) (res []string) {
	seen := make(map[string]bool)
	for _, re := range []*regexp.Regexp{urlRe, pathRe, attrRe} {
		for _, v := range extract(c, re, 1) {
			if !seen[v] {
				seen[v], res = true, append(res, v)
			}
		}
	}
	return
}

// ExtractRobots parses the contents of a robots.txt file to extract allowed/disallowed paths and sitemap URLs.
func ExtractRobots(c string) ([]string, []string) { return extract(c, robRe, 1), extract(c, rmapRe, 1) }

// ExtractSitemap parses the contents of a sitemap XML to extract all loc URLs.
func ExtractSitemap(c string) []string { return extract(c, smapRe, 1) }
