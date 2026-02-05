package cli

// countVerbosity counts -v flags and returns verbosity level and filtered args
func countVerbosity(args []string) (int, []string) {
	verbosity := 0
	filtered := make([]string, 0, len(args))

	for _, arg := range args {
		if arg == "-v" {
			verbosity++
		} else {
			filtered = append(filtered, arg)
		}
	}

	return verbosity, filtered
}
