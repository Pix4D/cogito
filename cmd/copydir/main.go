package main

import (
	"fmt"
	"os"
	"strings"

	arg "github.com/alexflint/go-arg"

	"github.com/Pix4D/cogito/help"
)

func main() {
	var args struct {
		Dot      bool     `arg:"help:rename dot.FOO to .FOO"`
		Template []string `arg:"help:template processing: key=val key=val ..."`
		Src      string   `arg:"positional,required,help:source directory"`
		Dst      string   `arg:"positional,required,help:destination directory"`
	}
	arg.MustParse(&args)
	templateData, err := makeTemplatelData(args.Template)
	if err != nil {
		fmt.Println("error:", err)
		os.Exit(1)
	}
	src := os.Args[1]
	dst := os.Args[2]
	fmt.Println("src:", src, "dst:", dst)

	var renamer help.Renamer
	if args.Dot {
		renamer = help.DotRenamer
	} else {
		renamer = help.IdentityRenamer
	}

	if err := help.CopyDir(dst, src, renamer, templateData); err != nil {
		fmt.Println("error:", err)
		os.Exit(1)
	}
}

// Take a list of strings of the form "key=value" and convert them to map entries.
func makeTemplatelData(keyvals []string) (help.TemplateData, error) {
	data := help.TemplateData{}
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
