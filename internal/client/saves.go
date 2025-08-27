package client

import (
	"net/http"
)

// Deprecated wrappers that keep the old function signatures but delegate to
// the new API type. Prefer constructing an *API and calling its methods.

// UploadSave uploads a local save file to the server using the provided
// server base URL.
func UploadSave(serverHTTP, localPath, player, game string) error {
	api := NewAPI(serverHTTP, http.DefaultClient, nil)
	return api.UploadSave(localPath, player, game)
}

// DownloadSave downloads a save file for player/filename into ./saves/player.
func DownloadSave(serverHTTP, player, filename string) error {
	api := NewAPI(serverHTTP, http.DefaultClient, nil)
	return api.DownloadSave(player, filename)
}
