package compiler

import "strconv"

// Infer body presence only when every known 2xx/3xx outcome requires a nonempty request stream.
func inferredBodyRequired(paths []flow) bool {
	accepted := false
	for _, path := range paths {
		status := responseSnapshot(path).Status
		code, err := strconv.Atoi(status)
		if err != nil || code < 200 || code > 599 {
			return false
		}
		if code >= 400 {
			continue
		}
		accepted = true
		required := false
		for _, effect := range path.effects {
			if effect.Kind == RequestBody || effect.Kind == RequestField {
				required = required || effect.NonEmptyBody
			}
		}
		if !required {
			return false
		}
	}
	return accepted
}

// Error-only path groups contribute response contracts without asserting optional successful input.
func rejectedBodyPaths(paths []flow) bool {
	if len(paths) == 0 {
		return false
	}
	for _, path := range paths {
		code, err := strconv.Atoi(responseSnapshot(path).Status)
		if err != nil || code < 400 || code > 599 {
			return false
		}
	}
	return true
}
