package main

// Ordinary text needs no raw payload or network enrichment. Inline forwards
// are also admitted here because they may contain nested ID-only forwards.
func needsMessageExpansion(msg any) bool {
	arr, _ := msg.([]any)
	for _, item := range arr {
		segment, _ := item.(map[string]any)
		if segment["type"] == "reply" || segment["type"] == "forward" {
			return true
		}
	}
	return false
}
