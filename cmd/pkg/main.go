// HERE FOR TESTING

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"time"

	// _ "net/http/pprof"

	"github.com/charlievieth/pkg"
)

func init() {
	// go func() {
	// 	log.Println(http.ListenAndServe("localhost:6060", nil))
	// }()
}

func main() {
	// WARN: attempt to not OOM
	runtime.GOMAXPROCS(4)

	c := pkg.NewCorpus()
	t := time.Now()
	if err := c.Init(); err != nil {
		Fatal(err)
	}
	d := time.Since(t)
	fmt.Println("update:", d)
	c.LogEvents = true
	time.Sleep(time.Hour)
}

func Fatal(err interface{}) {
	if err == nil {
		return
	}
	_, file, line, ok := runtime.Caller(1)
	if ok {
		file = filepath.Base(file)
	}
	switch err.(type) {
	case error, string, fmt.Stringer:
		if ok {
			fmt.Fprintf(os.Stderr, "Error (%s:%d): %s", file, line, err)
		} else {
			fmt.Fprintf(os.Stderr, "Error: %s", err)
		}
	default:
		if ok {
			fmt.Fprintf(os.Stderr, "Error (%s:%d): %#v\n", file, line, err)
		} else {
			fmt.Fprintf(os.Stderr, "Error: %#v\n", err)
		}
	}
	os.Exit(1)
}
