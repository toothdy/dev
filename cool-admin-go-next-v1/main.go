package main

import (
	"context"

	"github.com/gogf/gf/v2/frame/g"
	"github.com/toothdy/cool-admin-go-next/cool/app"
	"github.com/toothdy/cool-admin-go-next/modules"
)

/**
 * 应用入口
 */
func main() {
	if err := app.Run(context.Background(), modules.Specs()); err != nil {
		g.Log().Fatal(context.Background(), err)
	}
}
