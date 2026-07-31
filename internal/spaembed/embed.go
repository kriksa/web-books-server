// Package spaembed holds the Vite production build (web → internal/spaembed/spa).
package spaembed

import "embed"

//go:embed all:spa
var FS embed.FS
