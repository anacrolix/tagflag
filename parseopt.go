package tagflag

type parseOpt func(p *parser)

func BuiltinHelp() parseOpt {
	return func(p *parser) {
		p.builtinHelp = true
	}
}
