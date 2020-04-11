package models

import "net/url"

type ProxyEndpoint struct {
	URL  *url.URL
	Port int
}
