package main

import (
	"fmt"
	"github.com/alexcb/acbup/pack"
)

func main() {
	p := pack.New("/tmp/packroot")

	bkupPath := "/home/alex/music"
	err := p.AddDir(bkupPath)
	if err != nil {
		panic(err)
	}
	fmt.Printf("done\n")
}
