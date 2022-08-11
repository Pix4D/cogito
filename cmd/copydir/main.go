// Useful when developing testhelp.CopyDir.
package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/alexflint/go-arg"

	"github.com/Pix4D/cogito/testhelp"
)

func main() {
	var args struct {
		Dot      bool     `arg:"help:rename dot.FOO to .FOO"`
		Template []string `arg:"help:template processing: key=val key=val ..."`
		Src      string   `arg:"positional,required,help:source directory"`
		Dst      string   `arg:"positional,required,help:destination directory"`
	}
	arg.MustParse(&args)
	templateData, err := makeTemplateData(args.Template)
	if err != nil {
		fmt.Println("error:", err)
		os.Exit(1)
	}

	var renamer testhelp.Renamer
	if args.Dot {
		renamer = testhelp.DotRenamer
	} else {
		renamer = testhelp.IdentityRenamer
	}

	if err := testhelp.CopyDir(args.Dst, args.Src, renamer, templateData); err != nil {
		fmt.Println("error:", err)
		os.Exit(1)
	}
}

// Take a list of strings of the form "key=value" and convert them to map entries.
func makeTemplateData(keyvals []string) (testhelp.TemplateData, error) {
	data := testhelp.TemplateData{}
	for _, keyval := range keyvals {
		pos := strings.Index(keyval, "=")
		if pos == -1 {
			return data, fmt.Errorf("missing '=' in %s", keyval)
		}
		key := keyval[:pos]
		value := keyval[pos+1:]
		data[key] = value
	}
	return data, nil
}
