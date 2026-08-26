package web

import "embed"

// Dist holds the built Vue app (web/dist). A placeholder is committed so
// `go build` works before `npm run build`.
//
//go:embed all:dist
var Dist embed.FS
