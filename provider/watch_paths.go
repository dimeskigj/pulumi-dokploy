package dokploy

// normalizeWatchPaths maps an empty watchPaths to nil. A program that omits
// watchPaths yields nil while Dokploy reports the same resource with an empty
// list; both mean "no watch paths", so source comparisons must treat them alike.
func normalizeWatchPaths(paths []string) []string {
	if len(paths) == 0 {
		return nil
	}
	return paths
}
