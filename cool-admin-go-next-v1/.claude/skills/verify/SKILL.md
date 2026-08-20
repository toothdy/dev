# cool-admin-go-next Verify Skill

Use this when verifying runtime behavior for this GoFrame service.

## Launch

The default `:8001` port may already be occupied in local sessions. Use a temporary GoFrame config file and `GF_GCFG_FILE` to bind a safe port:

```bash
cat >/tmp/cool-go-verify.yaml <<'YAML'
server:
  address: ":18001"
  openapiPath: "/api.json"
  swaggerPath: "/swagger"
logger:
  level: "all"
  stdout: true
database:
  default:
    link: "mysql:root:123456@tcp(127.0.0.1:3306)/cool-go?loc=Local&parseTime=true&charset=utf8mb4"
    debug: true
cool:
  initDB: true
  initMenu: true
  initJudge: "db"
  schema:
    autoSync: true
    safeMode: true
    logDiff: true
  eps:
    enable: true
  auth:
    jwtSecret: "cool-admin-go-next-dev-secret"
    tokenExpire: 7200
    refreshExpire: 604800
YAML
GF_GCFG_FILE=/tmp/cool-go-verify.yaml go run .
```

Stop the background process after verification.

## Drive

- Health: `curl -sS http://127.0.0.1:18001/health`
- CRUD page: `curl -sS -X POST http://127.0.0.1:18001/admin/base/sys/user/page -H 'Content-Type: application/json' -d '{"page":1,"size":15}'`
- Probe unsafe sort: `curl -sS -X POST http://127.0.0.1:18001/admin/base/sys/user/page -H 'Content-Type: application/json' -d '{"sort":"username desc; drop table"}'`
- Probe wrong method: `curl -sS -i http://127.0.0.1:18001/admin/base/sys/user/page`

## Expected

- Health returns `code:1000` and `data.status:"ok"`.
- User page returns `code:1000`, `data.list`, and `data.pagination`.
- User page rows must not include `password`.
- Unsafe sort returns a business failure instead of executing arbitrary SQL.
