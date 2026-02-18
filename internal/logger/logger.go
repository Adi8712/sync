package logger

import "fmt"

const (
	R = "\033[31m"
	G = "\033[32m"
	Y = "\033[33m"
	B = "\033[34m"
	C = "\033[0m"
)

func Info(f string, a ...any) { fmt.Printf(B+C+f+"\n", a...) }
func Err(f string, a ...any)  { fmt.Printf(R+C+f+"\n", a...) }
func Done(f string, a ...any) { fmt.Printf(G+C+f+"\n", a...) }
func Warn(f string, a ...any) { fmt.Printf(Y+C+f+"\n", a...) }
