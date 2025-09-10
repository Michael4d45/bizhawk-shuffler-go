// Future: P2P save state fetcher implementation.
// The initial alpha relies solely on HTTP (see HTTPSaveStateFetcher). This file
// will later contain a composite fetcher that first attempts peer downloads
// using announced peers and then falls back to HTTP when unavailable.

package p2p
