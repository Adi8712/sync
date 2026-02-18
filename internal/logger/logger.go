package logger

import (
	"log"
	"os"
)

var (
	Info  *log.Logger
	Error *log.Logger
	Debug *log.Logger
)

func Init() {
	flags := log.Ldate | log.Ltime | log.Lmicroseconds | log.Lshortfile

	Info = log.New(os.Stdout, "[INFO]  ", flags)
	Error = log.New(os.Stdout, "[ERROR] ", flags)
	Debug = log.New(os.Stdout, "[DEBUG] ", flags)
}
