package tagflag

type parseOpt func(p *parser)

func NoDefaultHelp() parseOpt {
	return func(p *parser) {
		p.noDefaultHelp = true
	}
}

func Description(desc string) parseOpt {
	return func(p *parser) {
		p.description = desc
	}
}

func Program(name string) parseOpt {
	return func(p *parser) {
		p.program = name
	}
}
