package tagflag

type userError struct {
	msg string
}

func (ue userError) Error() string {
	return ue.msg
}

type logicError struct {
	msg string
}

func (le logicError) Error() string {
	return le.msg
}
