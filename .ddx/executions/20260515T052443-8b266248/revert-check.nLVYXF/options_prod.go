//go:build !testseam

package fizeau

// seamOptions is empty in production builds. The testseam build tag is
// required to construct any test injection seam. Embedding this struct into
// Options ensures that production builds cannot set seam fields — any attempt
// to do so is a compile error.
type seamOptions struct{}

func shouldAutoLoadServiceConfig(ServiceOptions) bool {
	return true
}
