package logging

// Logger defines logging operations following the Dependency Inversion Principle.
// Enables swapping logging implementations.
type Logger interface {
	InfoLog(format string, args ...interface{})
	DebugLog(format string, args ...interface{})
	ErrorLog(format string, args ...interface{})
	WarnLog(format string, args ...interface{})
}
