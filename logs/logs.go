package logs

import (
	"log"
	"os"
)

var (
	// Info Poor man's info level logger
	Info = log.New(os.Stdout, "[INFO] ", log.Ldate|log.Ltime)
	// Error Poor man's error level logger
	Error = log.New(os.Stderr, "[ERROR] ", log.Ldate|log.Ltime)
	// Warn Poor man's warn level logger
	Warn = log.New(os.Stdout, "[WARN] ", log.Ldate|log.Ltime)
)
