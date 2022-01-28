package consterr

//ConstErr is used to be able to declare constants that are errors that are strings
type ConstErr string

//Error returns the value of the underlying string
func (errstr ConstErr) Error() string { return string(errstr) }

//ErrNotImplemented can be used while things are under construction and serves
// as an example of how to use this simple package
const ErrNotImplemented = ConstErr("This feature is not implemented")

var _ error = ErrNotImplemented //compile time type check
