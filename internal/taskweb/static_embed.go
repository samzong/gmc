package taskweb

import "embed"

//go:embed static/index.html static/app.css static/app.js
var staticFiles embed.FS
