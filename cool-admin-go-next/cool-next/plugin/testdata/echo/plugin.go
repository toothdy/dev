package main

import (
	"context"
	"encoding/json"

	"github.com/toothdy/cool-admin-go-next/cool-next/plugin/sdk"
)

type echoConfig struct {
	Prefix string `json:"prefix"`
}

type echoRequest struct {
	Value string `json:"value"`
}

type echoResponse struct {
	Value string `json:"value"`
}

func init() {
	sdk.Register(sdk.Define(
		sdk.Method("echo", echo),
		sdk.RawMethod("host", callHost),
		sdk.RawMethod("panic", panicMethod),
		sdk.RawMethod("loop", loop),
	))
}

func echo(ctx context.Context, request echoRequest) (echoResponse, error) {
	config, err := sdk.Config[echoConfig](ctx)
	if err != nil {
		return echoResponse{}, err
	}

	return echoResponse{Value: config.Prefix + request.Value}, nil
}

func callHost(ctx context.Context, input json.RawMessage) (json.RawMessage, error) {
	return sdk.HostCall(ctx, "echo.prefix", input)
}

func panicMethod(context.Context, json.RawMessage) (json.RawMessage, error) {
	panic("guest panic")
}

func loop(context.Context, json.RawMessage) (json.RawMessage, error) {
	for {
	}
}
