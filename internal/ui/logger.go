package ui

import (
	"image/color"
	"sync"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/widget"
)

// UI color palette
var (
	ColorSuccess = color.NRGBA{R: 0, G: 255, B: 136, A: 255}
	colorError   = color.NRGBA{R: 255, G: 69, B: 96, A: 255}
	colorWarn    = color.NRGBA{R: 255, G: 200, B: 0, A: 255}
	colorInfo    = color.NRGBA{R: 136, G: 136, B: 200, A: 255}
	colorStep    = color.NRGBA{R: 100, G: 180, B: 255, A: 255}
	colorDim     = color.NRGBA{R: 100, G: 100, B: 130, A: 255}
)

type logEntry struct {
	text  string
	color color.Color
}

// Logger provides thread-safe colored logging to a Fyne list widget.
// Implements device.Logger interface.
type Logger struct {
	entries []logEntry
	mu      sync.Mutex
	list    *widget.List
}

func NewLogger() *Logger {
	l := &Logger{}
	l.list = widget.NewList(
		func() int {
			l.mu.Lock()
			defer l.mu.Unlock()
			return len(l.entries)
		},
		func() fyne.CanvasObject {
			t := canvas.NewText("", color.White)
			t.TextSize = 12
			t.TextStyle = fyne.TextStyle{Monospace: true}
			return t
		},
		func(id widget.ListItemID, obj fyne.CanvasObject) {
			l.mu.Lock()
			defer l.mu.Unlock()
			t := obj.(*canvas.Text)
			if id < len(l.entries) {
				t.Text = l.entries[id].text
				t.Color = l.entries[id].color
				t.Refresh()
			}
		},
	)
	return l
}

func (l *Logger) Widget() *widget.List { return l.list }

func (l *Logger) append(clr color.Color, msg string) {
	l.mu.Lock()
	l.entries = append(l.entries, logEntry{text: msg, color: clr})
	l.mu.Unlock()
	fyne.Do(func() {
		l.list.Refresh()
		l.list.ScrollToBottom()
	})
}

func (l *Logger) Info(msg string)    { l.append(colorInfo, msg) }
func (l *Logger) Success(msg string) { l.append(ColorSuccess, msg) }
func (l *Logger) Error(msg string)   { l.append(colorError, msg) }
func (l *Logger) Warn(msg string)    { l.append(colorWarn, msg) }
func (l *Logger) Step(msg string)    { l.append(colorStep, msg) }
func (l *Logger) Dim(msg string)     { l.append(colorDim, msg) }

func (l *Logger) Clear() {
	l.mu.Lock()
	l.entries = nil
	l.mu.Unlock()
	fyne.Do(func() { l.list.Refresh() })
}

func (l *Logger) CopyAll() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	var sb []byte
	for _, e := range l.entries {
		sb = append(sb, e.text...)
		sb = append(sb, '\n')
	}
	return string(sb)
}
