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
			if err != nil {
				return nil, err
			}

			if res.StatusCode < 200 || res.StatusCode >= 300 {
				defer res.Body.Close()
				body, _ := ioutil.ReadAll(io.LimitReader(res.Body, 512))
				snippet := string(body)
				if len(snippet) > 0 {
					return nil, fmt.Errorf("GET %s: %s: %s", url, res.Status, snippet)
				}
				return nil, fmt.Errorf("GET %s: %s", url, res.Status)
			}

			return res.Body, nil
		},
	}
}

func NewTester(expectations map[string]string) Getter {
	return &getter{
		responseBodyFor: func(url string, opts Opts) (io.ReadCloser, error) {
			res, ok := expectations[url]
			if !ok {
				return nil, fmt.Errorf("unexpected input: url=%v, opts=%v", url, opts)
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
