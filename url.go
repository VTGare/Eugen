package main

import "net/url"

type URLType int

const (
	URLTypeImage URLType = iota
	URLTypeVideo
	URLTypeTenor
	URLTypeImgur
)

func (t URLType) String() string {
	return [...]string{"Image", "Video", "Tenor", "Imgur"}[t]
}

type EugenURL struct {
	URL  *url.URL
	Type URLType
}
