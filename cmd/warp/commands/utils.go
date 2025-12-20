package commands

// countVerbosity counts how many -v or --verbose flags are in args
// Returns: verbosity level (0, 1, 2, 3+), filtered args without -v/--verbose
func countVerbosity(args []string) (int, []string) {
	verbosity := 0
	filtered := make([]string, 0, len(args))

	for _, arg := range args {
		if arg == "-v" || arg == "--verbose" {
			verbosity++
		} else {
			filtered = append(filtered, arg)
		}
	}

	return verbosity, filtered
}
