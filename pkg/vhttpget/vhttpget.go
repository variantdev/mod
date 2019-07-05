package vhttpget

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
)

type Option interface {
	Set(o *Opts)
}

type Opts struct {
}

type Getter interface {
	DoRequest(url string, opt ...Option) (string, error)
}

type getter struct {
	responseBodyFor func(url string, opts Opts) (io.ReadCloser, error)
}

func New() Getter {
	return &getter{
		responseBodyFor: func(url string, opts Opts) (io.ReadCloser, error) {
			res, err := http.Get(url)
			return res.Body, err
		},
	}
}

type TestGetInput struct {
	URL  string
	Opts Opts
}

func NewTester(expectations map[TestGetInput]string) Getter {
	return &getter{
		responseBodyFor: func(url string, opts Opts) (io.ReadCloser, error) {
			input := TestGetInput{URL: url, Opts: opts}
			res, ok := expectations[input]
			if !ok {
				return nil, fmt.Errorf("unexpected input: %v", input)
			}
			r := ioutil.NopCloser(bytes.NewReader([]byte(res)))
			return r, nil
		},
	}
}

func (t *getter) DoRequest(url string, opt ...Option) (string, error) {
	opts := &Opts{}
	for _, o := range opt {
		o.Set(opts)
	}

	res, err := t.responseBodyFor(url, *opts)
	if err != nil {
		return "", err
	}

	bytes, err := ioutil.ReadAll(res)
	if err != nil {
		return "", err
	}

	return string(bytes), nil
}
