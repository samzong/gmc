package cmd

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"strings"
)

var (
	outWriterFunc = func() io.Writer { return os.Stdout }
	errWriterFunc = func() io.Writer { return os.Stderr }
)

func init() {
	outWriterFunc = func() io.Writer { return rootCmd.OutOrStdout() }
	errWriterFunc = func() io.Writer { return rootCmd.ErrOrStderr() }
}

func outWriter() io.Writer {
	return outWriterFunc()
}

func errWriter() io.Writer {
	return errWriterFunc()
}

type outputFormatFlag struct {
	value string
}

func (f *outputFormatFlag) String() string { return f.value }
func (f *outputFormatFlag) Set(s string) error {
	if s != "text" && s != "json" {
		return errors.New("must be text or json")
	}
	f.value = s
	return nil
}
func (f *outputFormatFlag) Type() string { return "string" }

var outputFlag = &outputFormatFlag{value: "text"}

func outputFormat() string {
	return outputFlag.value
}

func printJSON(w io.Writer, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	_, err = w.Write(append(data, '\n'))
	return err
}

func sanitizeForTerminal(value string) string {
	return strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f {
			return -1
		}
		return r
	}, value)
}
