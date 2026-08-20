package main

import (
	"os"

	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gctx"

	"github.com/toothdy/cool-admin-go-next/cool-next/core/app"
	"github.com/toothdy/cool-admin-go-next/cool-next/core/exception"
	"github.com/toothdy/cool-admin-go-next/modules"
)

func main() {
	ctx := gctx.GetInitCtx()
	if err := app.Run(ctx, modules.Generated()); err != nil {
		g.Log().Error(ctx, exception.LogText(err))
		os.Exit(1)
	}
}
