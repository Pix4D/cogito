package main

import (
	"fmt"
	"os"

	"github.com/Pix4D/cogito/help"
)

func main() {
	if len(os.Args) != 3 {
		fmt.Println("usage: dotcopy src-dir dst-dir")
		os.Exit(1)
	}
	src := os.Args[1]
	dst := os.Args[2]

	fmt.Println("src:", src, "dst:", dst)
	err := help.CopyDir(dst, src, help.DotRenamer)
	fmt.Println(err)
}
