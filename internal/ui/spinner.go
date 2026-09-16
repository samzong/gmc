package ui

import (
	"os"
	"time"

	"github.com/briandowns/spinner"
	"github.com/mattn/go-isatty"
)

type Spinner struct {
	s *spinner.Spinner
}

func NewSpinner(message string) *Spinner {
	enabled := isatty.IsTerminal(os.Stderr.Fd()) || isatty.IsCygwinTerminal(os.Stderr.Fd())
	if !enabled {
		return &Spinner{}
	}

	s := spinner.New(spinner.CharSets[14], 100*time.Millisecond, spinner.WithWriter(os.Stderr))
	s.Suffix = " " + message
	return &Spinner{s: s}
}

func (sp *Spinner) Start() {
	if sp.s != nil {
		sp.s.Start()
	}
}

func (sp *Spinner) Stop() {
	if sp.s != nil {
		sp.s.Stop()
	}
}
