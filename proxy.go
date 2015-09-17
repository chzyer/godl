package main

import (
	"io"
	"net/http"
	"net/url"
	"strconv"

	"gopkg.in/logex.v1"
)

func bindHandler(mux *http.ServeMux) {
	mux.HandleFunc("/proxy", proxyHandler)
}

type ProxyConfig struct {
	Url   string
	Start int64
	End   int64
}

func proxyUrl(host, source string, start, end int64) string {
	u := url.Values{
		"url": {source},
	}
	if start >= 0 {
		u.Add("start", strconv.FormatInt(start, 10))
	}
	if end > 0 {
		u.Add("end", strconv.FormatInt(end, 10))
	}
	return "http://" + host + "/proxy?" + u.Encode()
}

func proxyDo(method string, p *ProxyConfig, h http.Header) (io.ReadCloser, int, error) {
	req, err := http.NewRequest(method, p.Url, nil)
	if err != nil {
		return nil, 400, logex.Trace(err)
	}
	if p.Start >= 0 {
		setRange(req.Header, p.Start, p.End)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, 400, logex.Trace(err)
	}
	for k, v := range resp.Header {
		for _, vv := range v {
			h.Add(k, vv)
		}
	}

	switch resp.StatusCode {
	case 206:
		return resp.Body, resp.StatusCode, nil
	case 200:
		// panic
		return resp.Body, resp.StatusCode, nil
	default:
		return nil, resp.StatusCode, logex.NewError("remote error:", resp.Status)
	}
}

func proxyHandler(w http.ResponseWriter, req *http.Request) {
	cfg := new(ProxyConfig)
	cfg.Url = req.FormValue("url")
	cfg.Start, _ = strconv.ParseInt(req.FormValue("start"), 10, 64)
	cfg.End, _ = strconv.ParseInt(req.FormValue("end"), 10, 64)

	rc, code, err := proxyDo(req.Method, cfg, w.Header())
	if err != nil {
		http.Error(w, err.Error(), code)
		return
	}
	defer rc.Close()
	_, err = io.Copy(w, rc)
	if err != nil {
		logex.Error(err)
		return
	}
}
