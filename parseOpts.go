package tagflag

type parseOpt func(p *parser)

// Exit the program with an error instead of returning it.
func ExitOnError() parseOpt {
	return func(p *parser) {
		p.exitOnError = true
	}
}

// Don't treat bad fields in the command struct as an error.
func SkipBadTypes() parseOpt {
	return func(p *parser) {
		p.skipUnsettable = true
	}
}

// Add -h, and --help flags that print usage to stdout and exit(0).
func HelpFlag() parseOpt {
	return func(p *parser) {
		p.printHelp = true
	}
}

// Sets the program name normally presumed to be the first argument shown in usage.
func Program(program string) parseOpt {
	return func(p *parser) {
		p.program = program
	}
}

// Writes program description between usage and option help.
func Description(desc string) parseOpt {
	return func(p *parser) {
		p.description = desc
	}
}
