package main

import (
	"fmt"
	"os"
	"encoding/json"

	"github.com/antchfx/htmlquery"
)

type Version struct {
	Number string `yaml:"number"`
}

func main() {
	doc, err := htmlquery.LoadURL("https://docs.aws.amazon.com/eks/latest/userguide/kubernetes-versions.html")
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}

	nodes := htmlquery.Find(doc, "//ul[1]/li/p")

	vs := []string{}
	for _, n := range nodes {
		vs = append(vs, n.FirstChild.Data)
	}

	vsJson, err := json.Marshal(vs)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
	}

	fmt.Printf("%s\n", vsJson)
}
