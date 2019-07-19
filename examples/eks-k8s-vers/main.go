package main

import (
	"fmt"
	"os"

	"github.com/antchfx/htmlquery"
)

func main() {
	doc, err := htmlquery.LoadURL("https://docs.aws.amazon.com/eks/latest/userguide/kubernetes-versions.html")
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}

	nodes := htmlquery.Find(doc, "//ul[1]/li/p")

	for _, n := range nodes {
		fmt.Printf("%s\n", n.FirstChild.Data)
	}
}
