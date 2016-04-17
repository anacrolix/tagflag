package tagflag

type parseOpt func(p *parser)

func NoDefaultHelp() parseOpt {
	return func(p *parser) {
		p.noDefaultHelp = true
	}
}
