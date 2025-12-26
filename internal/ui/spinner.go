package ui

import (
	"os"
	"time"

	"github.com/briandowns/spinner"
	"github.com/mattn/go-isatty"
)

// Spinner wraps briandowns/spinner with TTY awareness
type Spinner struct {
	s       *spinner.Spinner
	enabled bool
}

// NewSpinner creates a new spinner that only displays on TTY
func NewSpinner(message string) *Spinner {
	// Only enable on TTY
	enabled := isatty.IsTerminal(os.Stderr.Fd()) || isatty.IsCygwinTerminal(os.Stderr.Fd())
	if !enabled {
		return &Spinner{enabled: false}
	}

	s := spinner.New(spinner.CharSets[14], 100*time.Millisecond, spinner.WithWriter(os.Stderr))
	s.Suffix = " " + message
	return &Spinner{s: s, enabled: true}
}

// Start begins the spinner animation
func (sp *Spinner) Start() {
	if sp.enabled && sp.s != nil {
		sp.s.Start()
	}
}

// Stop ends the spinner animation
func (sp *Spinner) Stop() {
	if sp.enabled && sp.s != nil {
		sp.s.Stop()
	}
}

// UpdateMessage changes the spinner message
func (sp *Spinner) UpdateMessage(message string) {
	if sp.enabled && sp.s != nil {
		sp.s.Suffix = " " + message
	}
}
