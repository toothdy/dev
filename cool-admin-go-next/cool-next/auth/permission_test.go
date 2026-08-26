package auth

import "testing"

func TestDerivePermission(t *testing.T) {
	cases := []struct {
		name        string
		path        string
		ignoreToken bool
		want        string
		wantErr     bool
	}{
		{name: "CRUD 路由", path: "/admin/base/sys/department/add", want: "base:sys:department:add"},
		{name: "自定义路由", path: "/admin/base/sys/user/move", want: "base:sys:user:move"},
		{name: "驼峰路径段", path: "/admin/base/sys/log/getKeep", want: "base:sys:log:getKeep"},
		{name: "工具路由", path: "/admin/base/coding/createCode", want: "base:coding:createCode"},

		{name: "Go 关键字段 import", path: "/admin/base/sys/menu/import", want: "base:sys:menu:import"},
		{name: "Go 关键字段 type", path: "/admin/dict/type/add", want: "dict:type:add"},
		{name: "Go 关键字段 interface", path: "/admin/a/interface/b", want: "a:interface:b"},

		{name: "ignoreToken 放行", path: "/admin/base/open/captcha", ignoreToken: true, want: ""},
		{name: "ignoreToken 优先于推导", path: "/admin/base/sys/user/add", ignoreToken: true, want: ""},

		{name: "comm 位于中段", path: "/admin/base/comm/person", want: ""},
		{name: "comm 紧随 admin", path: "/admin/comm/upload", want: ""},
		{name: "comm 深层嵌套", path: "/admin/a/b/comm/c/d", want: ""},
		{name: "comm 位于末段不豁免", path: "/admin/base/comm", want: "base:comm"},
		{name: "comm 作为前缀不豁免", path: "/admin/base/common/list", want: "base:common:list"},

		{name: "App 路由", path: "/app/base/comm/upload", want: ""},
		{name: "静态文件路由", path: "/upload/{date}/{name}", ignoreToken: true, want: ""},
		{name: "非 admin 含路径参数", path: "/upload/{date}/{name}", want: ""},
		{name: "admin 前缀但非路径边界", path: "/administrator/x", want: ""},

		{name: "admin 下含路径参数", path: "/admin/base/file/{name}", wantErr: true},
		{name: "admin 下含连字符", path: "/admin/base/sys/user-list", wantErr: true},
		{name: "admin 下数字开头", path: "/admin/base/2fa", wantErr: true},
		{name: "仅 admin 前缀", path: "/admin/", wantErr: true},
		{name: "admin 下空段", path: "/admin/base//add", wantErr: true},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			got, err := DerivePermission(testCase.path, testCase.ignoreToken)
			if testCase.wantErr {
				if err == nil {
					t.Fatalf("期望推导失败，实际得到 %q", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("推导失败: %v", err)
			}
			if got != testCase.want {
				t.Fatalf("推导结果 = %q，期望 %q", got, testCase.want)
			}
		})
	}
}
