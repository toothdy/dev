package dto

// 权限菜单响应 data
type PermMenuResult struct {
   Menus []map[string]interface{} `json:"menus"`
   Perms []string                 `json:"perms"`
}