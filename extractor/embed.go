// Package extractor embeds the Node.js extractor script.
package extractor

import _ "embed"

//go:embed index.js
var Script []byte
