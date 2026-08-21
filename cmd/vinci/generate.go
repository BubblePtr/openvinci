package main

import "io"

func resolveFormat(opts *options) *cliError {
	return nil
}

func generate(opts *options, env func(string) string, stdout, stderr io.Writer) *cliError {
	return errorf(codeAPIError, exitGeneric, "not implemented")
}
