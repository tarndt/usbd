package errs //Bare bones error handling library that extends Go error handling

import (
	"bytes"
	"fmt"
	"path"
	"runtime"
	"runtime/debug"
)

var _ error = &Error{}

type Error struct {
	Msg    string
	Parent error
}

func (this *Error) Error() string {
	return this.Msg
}

func New(errMsg string, args ...interface{}) error {
	name, file, line := getCallersInfo(2)
	errMsg = fmtErrMsg(errMsg, args)
	return &Error{Msg: fmt.Sprintf("%s:%d %s(): %s", file, line, name, errMsg)}
}

func Append(suppliedErr error, errMsg string, args ...interface{}) error {
	if suppliedErr == nil {
		return New(errMsg, args)
	}
	name, file, line := getCallersInfo(2)
	errMsg = fmtErrMsg(errMsg, args)
	return &Error{
		Msg:    fmt.Sprintf("%s:%d %s(): %s;\n\tDetails: %s", file, line, name, errMsg, suppliedErr),
		Parent: suppliedErr,
	}
}

func PanicToErr(recoverReturn interface{}) error {
	const msgFmt = "%s:%d %s(): A panic occured, but was recovered; Details: %+v;\n\t*** Stack Trace ***\n\t%s\n"
	if recoverReturn != nil {
		name, file, line := getCallersInfo(4)
		prettyStack := bytes.Join(bytes.Split(debug.Stack(), []byte{'\n'})[6:], []byte{'\n', '\t'})
		return fmt.Errorf(msgFmt, file, line, name, recoverReturn, prettyStack)
	}
	return nil
}

func fmtErrMsg(msg string, args []interface{}) string {
	if len(msg) == 0 {
		msg = "An unexpected error occured"
	} else if len(args) > 0 {
		msg = fmt.Sprintf(msg, args...)
	}
	return msg
}

func getCallersInfo(depth int) (name, file string, line int) {
	name, file, line = "Unknown Function", "Unknown File", -1
	var programCounter uintptr
	var knownFunction bool
	if programCounter, file, line, knownFunction = runtime.Caller(depth); knownFunction {
		file = path.Base(file)
		name = path.Base(runtime.FuncForPC(programCounter).Name())
	}
	return
}
