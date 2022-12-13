package amimaker

const (
	errFilterFail = errType(iota)
	errFatal
)

type errType int

type handlerErr struct {
	errType errType
	err     error
}

func (o handlerErr) Error() string {
	return o.err.Error()
}

func (o handlerErr) Type() errType {
	return o.errType
}
