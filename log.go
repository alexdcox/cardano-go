package main

import (
	"fmt"
	"os"
	"time"

	"github.com/pkg/errors"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/pkgerrors"
)

var log = zerolog.New(nil).Output(zerolog.ConsoleWriter{
	Out:        os.Stderr,
	TimeFormat: time.TimeOnly,
}).With().Timestamp().Logger()

func Logger() zerolog.Logger {
	return log
}

func init() {
	zerolog.TimeFieldFormat = time.TimeOnly
	zerolog.ErrorStackMarshaler = MarshalStack
	zerolog.SetGlobalLevel(zerolog.InfoLevel)
}

func MarshalStack(err error) interface{} {
	fmt.Println(StackTracerMessage(err))
	return pkgerrors.MarshalStack(err)
}

func StackTracerMessage(err error) string {
	type StackTracer interface {
		StackTrace() errors.StackTrace
	}

	var errString string

	if err != nil {
		if stackTracer, isStackTracer := err.(StackTracer); isStackTracer {
			for _, f := range stackTracer.StackTrace() {
				errString += fmt.Sprintf("%+v\n", f)
			}
		}
	}

	return errString
}
