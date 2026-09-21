// Package steamapi calls Steam's public web APIs - Workshop item metadata
// (ISteamRemoteStorage/GetPublishedFileDetails) and Store app details
// (store.steampowered.com/api/appdetails) - both unauthenticated, no API
// key needed, confirmed by direct real requests during development (see
// docs/steam-web-api.md). Workshop metadata can also be fetched with the
// user's own Steam Web API key (IPublishedFileService/GetDetails, see keyed.go
// and Service), which returns items the free endpoint cannot, such as unlisted
// ones. Apart from the optional GitHub background images, this is the only
// package in this project that makes outbound network calls to a third party;
// everything else works entirely from local files.
package steamapi

import (
	"net/http"
	"time"
)

// httpClient is shared by every request in this package - a bounded
// timeout matters here specifically because it's the one place a slow or
// unresponsive third party could otherwise hang a call indefinitely,
// unlike every other data source in this project (all local disk reads).
var httpClient = &http.Client{Timeout: 15 * time.Second}
