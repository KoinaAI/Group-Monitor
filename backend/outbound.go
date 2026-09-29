package main

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

func validateHTTPURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return fmt.Errorf("invalid HTTP URL")
	}
	if u.User != nil || u.Fragment != "" {
		return fmt.Errorf("HTTP URL must not contain credentials or fragment")
	}
	return nil
}

func noRedirectClient(base *http.Client) *http.Client {
	client := *base
	client.CheckRedirect = func(_ *http.Request, _ []*http.Request) error {
		return http.ErrUseLastResponse
	}
	return &client
}

func httpHost(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	return strings.ToLower(u.Host)
}
