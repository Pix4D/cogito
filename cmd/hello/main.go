// This executable is here only to do basic tests in the Docker container.
package main

import (
	"fmt"

	"github.com/Pix4D/cogito/resource"
)

func main() {
	fmt.Println("hello")
	fmt.Println(resource.VersionString())
}
