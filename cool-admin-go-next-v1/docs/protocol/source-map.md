# Base Protocol Source Map

日期：2026-07-14

## 前端来源

| 契约 | 文件 |
|---|---|
| axios 响应解包、错误处理、Authorization header | `/Users/n/数据/cool-admin/cool-admin-vue/src/cool/service/request.ts` |
| token / refreshToken 本地存储与刷新调用 | `/Users/n/数据/cool-admin/cool-admin-vue/src/modules/base/store/user.ts` |
| 权限菜单消费结构 | `/Users/n/数据/cool-admin/cool-admin-vue/src/modules/base/store/menu.ts` |
| EPS service 注入方式 | `/Users/n/数据/cool-admin/cool-admin-vue/src/cool/bootstrap/eps.ts` |

## Node 来源

| 契约 | 文件 |
|---|---|
| open 接口：eps/html/login/captcha/refreshToken | `/Users/n/数据/cool-admin/cool-admin-midway/src/modules/base/controller/admin/open.ts` |
| comm 接口：person/personUpdate/permmenu/upload/uploadMode/logout/program | `/Users/n/数据/cool-admin/cool-admin-midway/src/modules/base/controller/admin/comm.ts` |
| 登录、验证码、刷新 Token、JWT payload、密码 md5 | `/Users/n/数据/cool-admin/cool-admin-midway/src/modules/base/service/sys/login.ts` |
| 权限菜单返回 `{ perms, menus }` | `/Users/n/数据/cool-admin/cool-admin-midway/src/modules/base/service/sys/perms.ts` |
