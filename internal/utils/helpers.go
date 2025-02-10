package utils

import (
	"fmt"
	"os"
	"reflect"
)

func PanicIfErr(e error) {
	if e != nil {
		fmt.Fprintf(os.Stderr, "error type: %s\n", reflect.TypeOf(e))
		panic(e)
	}
}
