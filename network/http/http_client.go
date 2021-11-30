package http

import (
	"errors"
	"time"

	"github.com/blockpilabs/solana-drpc/log"
	"github.com/valyala/fasthttp"
)

const TIMEOUT = time.Second * 5

var logger = log.GetLogger("httpclient")
var client = &fasthttp.Client{
	MaxConnsPerHost: 1024,
}

func PostJson(url string, jsonStr string) (body []byte, err error) {
	req := &fasthttp.Request{}
	req.SetRequestURI(url)
	requestBody := []byte(jsonStr)
	req.SetBody(requestBody)
	req.Header.SetContentType("application/json")
	req.Header.SetMethod("POST")
	resp := &fasthttp.Response{}

	if err := client.DoTimeout(req, resp, TIMEOUT);err != nil {
		//logger.Debug(err)
		return nil, errors.New(err.Error() + " " + url)
	}
	return resp.Body(), nil
}