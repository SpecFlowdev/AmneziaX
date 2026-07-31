// Package scripts embeds the installers so the panel can serve the node
// installer over HTTP. Keeping the canonical copies here means the shipped
// script and the one in the repository can never drift apart.
package scripts

import _ "embed"

//go:embed install-node.sh
var InstallNode string

//go:embed install-panel.sh
var InstallPanel string
