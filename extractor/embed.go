// Package extractor embeds the Node.js extractor scripts.
package extractor

import _ "embed"

//go:embed index.js
var Script []byte

//go:embed runtime.js
var RuntimeScript []byte
