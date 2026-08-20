package dto

// 菜单导出节点
type MenuExportItem struct {
	TenantID   *int64           `json:"tenantId"`
	Name       string           `json:"name"`
	Router     *string          `json:"router"`
	Perms      *string          `json:"perms"`
	Type       int              `json:"type"`
	Icon       *string          `json:"icon"`
	OrderNum   int              `json:"orderNum"`
	ViewPath   *string          `json:"viewPath"`
	KeepAlive  bool             `json:"keepAlive"`
	IsShow     bool             `json:"isShow"`
	ChildMenus []MenuExportItem `json:"childMenus"`
}

// 菜单导入节点
type MenuImportItem struct {
	TenantID   *int64           `json:"tenantId"`
	Name       string           `json:"name"`
	Router     *string          `json:"router"`
	Perms      *string          `json:"perms"`
	Type       *int             `json:"type"`
	Icon       *string          `json:"icon"`
	OrderNum   *int             `json:"orderNum"`
	ViewPath   *string          `json:"viewPath"`
	KeepAlive  *bool            `json:"keepAlive"`
	IsShow     *bool            `json:"isShow"`
	ChildMenus []MenuImportItem `json:"childMenus"`
}

// 菜单导出请求
type MenuExportReq struct {
	IDs []int64 `json:"ids" v:"required#菜单不能为空"`
}

// 菜单导入请求
type MenuImportReq struct {
	Menus []MenuImportItem `json:"menus" v:"required#导入菜单不能为空"`
}

// 菜单实体解析请求
type MenuParseReq struct {
	Entity     string `json:"entity"`
	Controller string `json:"controller"`
	Module     string `json:"module"`
}

// 菜单实体字段元数据
type MenuParseColumn struct {
	PropertyName string `json:"propertyName"`
	Type         string `json:"type"`
	Length       string `json:"length"`
	Comment      string `json:"comment"`
	Nullable     bool   `json:"nullable"`
}

// 菜单实体解析结果
type MenuParseRes struct {
	Columns   []MenuParseColumn `json:"columns"`
	ClassName string            `json:"className,omitempty"`
	TableName string            `json:"tableName,omitempty"`
	FileName  string            `json:"fileName,omitempty"`
	Path      string            `json:"path"`
}
