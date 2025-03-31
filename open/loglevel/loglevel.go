package loglevel

import (
	"log"
	"os"
)

var (
	infoLogger    *log.Logger
	successLogger *log.Logger
	warnLogger    *log.Logger
	errLogger     *log.Logger
)

func Init() {
	infoLogger = log.New(os.Stdout, "[INFO] ", log.Ldate|log.Ltime|log.Lshortfile)
	successLogger = log.New(os.Stdout, "[SUCCESS] ", log.Ldate|log.Ltime|log.Lshortfile)
	warnLogger = log.New(os.Stderr, "[WARNING] ", log.Ldate|log.Ltime|log.Lshortfile)
	errLogger = log.New(os.Stderr, "[ERROR] ", log.Ldate|log.Ltime|log.Lshortfile)
}

func GetInfoLogger() *log.Logger {
	if infoLogger == nil {
		Init()
	}
	return infoLogger
}

func GetSuccessLogger() *log.Logger {
	if successLogger == nil {
		Init()
	}
	return successLogger
}

func GetWarnLogger() *log.Logger {
	if warnLogger == nil {
		Init()
	}
	return warnLogger
}

func GetErrLogger() *log.Logger {
	if errLogger == nil {
		Init()
	}
	return errLogger
}
