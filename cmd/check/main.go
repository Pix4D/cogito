package main

import (
	"github.com/Pix4D/cogito/resource"
	"github.com/cloudboss/ofcourse/ofcourse"
)

func main() {
	ofcourse.Check(resource.New())
}
