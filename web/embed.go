// Package web embeds the compiled local reading interface in the Hub binary.
package web

import "embed"

//go:embed all:dist
var Files embed.FS
