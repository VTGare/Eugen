package embed

import "net/url"

// URLType represents the type of a parsed URL.
type URLType int

const (
	URLTypeImage URLType = iota
	URLTypeVideo
	URLTypeImgur
)

// EugenURL wraps a parsed URL with its type classification.
type EugenURL struct {
	URL  *url.URL
	Type URLType
}
