package http_upstream

import (
	"encoding/json"
	"errors"
	"time"

	"github.com/blockpilabs/solana-drpc/common"
	"github.com/blockpilabs/solana-drpc/log"
	httpReq "github.com/blockpilabs/solana-drpc/network/http"
	"github.com/blockpilabs/solana-drpc/plugins/load_balancer"
	"github.com/blockpilabs/solana-drpc/rpc"
)

var logger = log.GetLogger("http_upstream")

type HttpUpstreamMiddleware struct {
	options *httpUpstreamMiddlewareOptions
	loadBalancer *load_balancer.LoadBalanceMiddleware
}

func NewHttpUpstreamMiddleware(argOptions ...common.Option) *HttpUpstreamMiddleware {
	mOptions := &httpUpstreamMiddlewareOptions{
		upstreamTimeout:       30 * time.Second,
		defaultTargetEndpoint: "",
	}
	for _, o := range argOptions {
		o(mOptions)
	}
	m := &HttpUpstreamMiddleware{
		options:           	mOptions,
		loadBalancer:		load_balancer.NewLoadBalanceMiddleware(),
	}
	return m
}


func (m *HttpUpstreamMiddleware) Name() string {
	return "http-upstream"
}

func (m *HttpUpstreamMiddleware) OnStart() (err error) {
	logger.Info("http upstream plugin starting")
	return nil
}



func (m *HttpUpstreamMiddleware) AddRpcEndPoint(group, endpoint string, weight int64) {

	//load_balancer.NewLoadBalanceMiddleware()

	m.loadBalancer.AddUpstreamItem(group, load_balancer.NewUpstreamItem(endpoint, weight))
}

func (m *HttpUpstreamMiddleware) OnConnection(session *rpc.ConnectionSession) (err error) {
	m.loadBalancer.OnConnection(session)

	//logger.Debugln("http upstream plugin on new connection")
	return nil
}

func (m *HttpUpstreamMiddleware) OnConnectionClosed(session *rpc.ConnectionSession) (err error) {
	// call next first
	m.loadBalancer.OnConnectionClosed(session)
	//logger.Debugln("http upstream plugin on connection closed")
	return nil
}


func (m *HttpUpstreamMiddleware) getTargetEndpoint(session *rpc.ConnectionSession) (target string, err error) {
	return GetSelectedUpstreamTargetEndpoint(session, &m.options.defaultTargetEndpoint)
}

func (m *HttpUpstreamMiddleware) OnRpcRequest(session *rpc.JSONRpcRequestSession) (err error) {

	m.loadBalancer.OnRpcRequest(session)

	targetEndpoint, err := m.getTargetEndpoint(session.Conn)
	if err != nil {
		return
	}
	//logger.Debugln("http stream receive rpc request for backend " + targetEndpoint)
	session.TargetServer = targetEndpoint
	// create response future before to use in ProcessRpcRequest
	session.RpcResponseFutureChan = make(chan *rpc.JSONRpcResponse, 1)
	rpcRequest := session.Request
	rpcRequestBytes, err := json.Marshal(rpcRequest)
	if err != nil {
		logger.Debugln("http rpc request format error", err.Error())
		errResp := rpc.NewJSONRpcResponse(rpcRequest.Id, nil,
			rpc.NewJSONRpcResponseError(rpc.RPC_INTERNAL_ERROR, err.Error(), nil))
		session.RpcResponseFutureChan <- errResp
		return
	}
	//logger.Debugln("rpc request " + string(rpcRequestBytes))

	httpRpcCall := func() (rpcRes *rpc.JSONRpcResponse, err error) {
		/*
		resp, err := http.Post(targetEndpoint, "application/json", bytes.NewReader(rpcRequestBytes))
		if err != nil {
			logger.Debugln("http rpc response error", err.Error())
			errResp := rpc.NewJSONRpcResponse(rpcRequest.Id, nil,
				rpc.NewJSONRpcResponseError(rpc.RPC_UPSTREAM_CONNECTION_CLOSED_ERROR, err.Error(), nil))
			session.RpcResponseFutureChan <- errResp
			return
		}
		defer resp.Body.Close()

		respMsg, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			return
		}
		*/

		respMsg, err := httpReq.PostJson(targetEndpoint, string(rpcRequestBytes))
		if err != nil {
			return
		}

		//logger.Debugln("backend rpc response " + string(respMsg))
		rpcRes, err = rpc.DecodeJSONRPCResponse(respMsg)
		if err != nil {
			return
		}
		if rpcRes == nil {
			err = errors.New("invalid jsonrpc response format from http upstream: " + string(respMsg))
			return
		}
		return
	}

	go func() {
		rpcRes, err := httpRpcCall()
		if err != nil {
			logger.Debugln("http rpc response error", targetEndpoint, err.Error())
			errResp := rpc.NewJSONRpcResponse(rpcRequest.Id, nil,
				rpc.NewJSONRpcResponseError(rpc.RPC_UPSTREAM_CONNECTION_CLOSED_ERROR, err.Error(), nil))
			session.RpcResponseFutureChan <- errResp
			return
		}
		session.RpcResponseFutureChan <- rpcRes
	}()

	return
}

func (m *HttpUpstreamMiddleware) OnRpcResponse(session *rpc.JSONRpcRequestSession) (err error) {
	m.loadBalancer.OnRpcResponse(session)
	return nil
}

func (m *HttpUpstreamMiddleware) ProcessRpcRequest(session *rpc.JSONRpcRequestSession) (err error) {
	m.loadBalancer.ProcessRpcRequest(session)

	if session.Response != nil {
		return
	}
	rpcRequest := session.Request
	rpcRequestId := rpcRequest.Id
	requestChan := session.RpcResponseFutureChan
	if requestChan == nil {
		err = errors.New("can't find rpc request channel to process")
		return
	}

	var rpcRes *rpc.JSONRpcResponse
	select {
	case <-time.After(m.options.upstreamTimeout):
		rpcRes = rpc.NewJSONRpcResponse(rpcRequestId, nil,
			rpc.NewJSONRpcResponseError(rpc.RPC_UPSTREAM_CONNECTION_CLOSED_ERROR,
				"upstream target connection closed", nil))
	case <-session.Conn.UpstreamTargetConnectionDone:
		rpcRes = rpc.NewJSONRpcResponse(rpcRequestId, nil,
			rpc.NewJSONRpcResponseError(rpc.RPC_UPSTREAM_CONNECTION_CLOSED_ERROR,
				"upstream target connection closed", nil))
	case rpcRes = <-requestChan:
		// do nothing, just receive rpcRes
	}

	session.Response = rpcRes
	return
}


func GetSelectedUpstreamTargetEndpoint(session *rpc.ConnectionSession, defaultValue *string) (result string, err error) {
	var defaultValueStr = ""
	if defaultValue != nil {
		defaultValueStr = *defaultValue
	}
	if session.SelectedUpstreamTarget == nil {
		result = defaultValueStr
		return
	}
	result = *session.SelectedUpstreamTarget
	return
}
