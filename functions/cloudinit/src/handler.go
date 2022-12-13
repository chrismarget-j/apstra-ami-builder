package cloudinit

import (
	"context"
	"errors"
	"fmt"
)

const (
	MessagePing      = "ping"
	MessagePong      = "pong"
	MessageSuccess   = "success"
	OperationInstall = "install"
)

type Request struct {
	InstanceIp string `json:"instance_ip"`
	Operation  string `json:"operation"`
}

type Response struct {
	Message string `json:"message,omitempty"`
	Error   string `json:"error,omitempty"`
}

func HandleRequest(ctx context.Context, request *Request) (*Response, error) {
	if request == nil {
		err := errors.New("error: nil Request")
		return &Response{
			Error: err.Error(),
		}, err
	}

	switch request.Operation {
	case MessagePing:
		return &Response{
			Message: MessagePong,
		}, nil
	default:
		err := fmt.Errorf("unknown operation: '%s'", request.Operation)
		return &Response{Message: err.Error()}, err
	}

	return nil, nil
}
