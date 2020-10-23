// HERE FOR TESTING

package main

import (
	"encoding/json"
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

func loadAndDump(filename string) error {
	f, err := os.OpenFile(filename, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0666)
	if err != nil {
		return err
	}
	defer f.Close()

	c := pkg.NewCorpus()
	c.IndexGoCode = true
	c.Update()

	enc := json.NewEncoder(f)
	enc.SetIndent("", "    ")
	if err := enc.Encode(c.Packages()); err != nil {
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return nil
}

func main() {
	if os.Args[1] == "dump" {
		if err := loadAndDump(os.Args[2]); err != nil {
			Fatal(err)
		}
		return
	}

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
