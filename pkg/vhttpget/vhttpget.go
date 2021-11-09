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
	Header map[string]string
}

func (o Opts) Set(another *Opts) {
	*another = o
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
			req, err := http.NewRequest(http.MethodGet, url, &bytes.Buffer{})
			if err != nil {
				return nil, err
			}

			if header := opts.Header; header != nil {
				for k, v := range header {
					req.Header.Add(k, v)
				}
			}

			res, err := http.DefaultClient.Do(req)
			return res.Body, err
		},
	}
}

type TestGetInput struct {
	URL  string
	Opts Opts
}

func (i TestGetInput) Key() string {
	return i.URL
}

type TestGetInputInterface interface {
	Key() string
}

func NewTester(expectations map[TestGetInputInterface]string) Getter {
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
