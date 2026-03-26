package device

// Logger is the interface used by device operations for output.
type Logger interface {
	Info(msg string)
	Success(msg string)
	Error(msg string)
	Warn(msg string)
	Step(msg string)
	Dim(msg string)
}
