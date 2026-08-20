package sys

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/toothdy/cool-admin-go-next/cool-next/auth"
	"github.com/toothdy/cool-admin-go-next/cool-next/core/controller"
	coreentity "github.com/toothdy/cool-admin-go-next/cool-next/core/entity"
	coreservice "github.com/toothdy/cool-admin-go-next/cool-next/core/service"
	"github.com/toothdy/cool-admin-go-next/modules/base/dto"
	"github.com/toothdy/cool-admin-go-next/modules/base/entity"
	"github.com/toothdy/cool-admin-go-next/modules/base/service"
)

// UserHandler 适配用户管理自定义接口。
type UserHandler struct {
	user *service.UserService
}

// UserPageReq 是用户分页固定筛选请求。
type UserPageReq struct {
	Page          int      `json:"page"`
	Size          int      `json:"size"`
	DepartmentIDs []uint64 `json:"departmentIds"`
	KeyWord       string   `json:"keyWord"`
	Status        *int32   `json:"status"`
}

// UserAddReq 是新增用户及其角色关系的请求。
type UserAddReq struct {
	DepartmentID *uint64  `json:"departmentId"`
	Name         *string  `json:"name"`
	Username     string   `json:"username" v:"required"`
	Password     string   `json:"password" v:"required"`
	NickName     *string  `json:"nickName"`
	HeadImg      *string  `json:"headImg"`
	Phone        *string  `json:"phone"`
	Email        *string  `json:"email"`
	Remark       *string  `json:"remark"`
	Status       *int32   `json:"status"`
	RoleIDList   []uint64 `json:"roleIdList" v:"required"`
}

// UserUpdateReq 是更新用户及其角色关系的请求。
type UserUpdateReq struct {
	ID           uint64    `json:"id" v:"required"`
	DepartmentID *uint64   `json:"departmentId"`
	Name         *string   `json:"name"`
	Username     *string   `json:"username"`
	Password     *string   `json:"password"`
	NickName     *string   `json:"nickName"`
	HeadImg      *string   `json:"headImg"`
	Phone        *string   `json:"phone"`
	Email        *string   `json:"email"`
	Remark       *string   `json:"remark"`
	Status       *int32    `json:"status"`
	RoleIDList   *[]uint64 `json:"roleIdList"`
	submitted    map[string]bool
}

// UnmarshalJSON 严格解码并记录更新字段是否提交。
func (request *UserUpdateReq) UnmarshalJSON(data []byte) error {
	type plain UserUpdateReq
	var value plain
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&value); err != nil {
		return err
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	*request = UserUpdateReq(value)
	request.submitted = make(map[string]bool, len(fields))
	for name := range fields {
		request.submitted[name] = true
	}

	return nil
}

// NewUserHandler 创建用户管理接口适配器。
func NewUserHandler(user *service.UserService) *UserHandler {
	return &UserHandler{user: user}
}

// Add 新增用户并写入角色关系。
func (handler *UserHandler) Add(ctx context.Context, request *UserAddReq) (coreservice.AddResult[uint64], error) {
	identity, err := auth.Admin(ctx)
	if err != nil {
		return coreservice.AddResult[uint64]{}, err
	}
	input, err := request.input(handler.user.Descriptor(), identity.UserID)
	if err != nil {
		return coreservice.AddResult[uint64]{}, err
	}

	return handler.user.AddWithRoles(ctx, input, request.RoleIDList)
}

// Update 更新用户并按提交状态替换角色关系。
func (handler *UserHandler) Update(ctx context.Context, request *UserUpdateReq) error {
	input, roleIDs, err := request.input(handler.user.Descriptor())
	if err != nil {
		return err
	}

	return handler.user.UpdateWithRoles(ctx, input, roleIDs)
}

// Move 批量移动用户部门。
func (handler *UserHandler) Move(ctx context.Context, request *dto.UserMoveReq) error {
	return handler.user.Move(ctx, *request)
}

// Page 返回当前管理员数据范围内的用户分页。
func (handler *UserHandler) Page(ctx context.Context, request *UserPageReq) (service.UserPageResult, error) {
	page := request.Page
	if page == 0 {
		page = 1
	}
	size := request.Size
	if size == 0 {
		size = 15
	}

	return handler.user.Page(ctx, page, size, service.UserPageFilter{
		DepartmentIDs: request.DepartmentIDs,
		KeyWord:       request.KeyWord,
		Status:        request.Status,
	})
}

func (request *UserAddReq) input(
	descriptor coreentity.Descriptor[entity.User, uint64],
	userID uint64,
) (coreservice.AddInput[entity.User], error) {
	fields := []coreservice.FieldValue{
		coreservice.Value("userId", userID),
		coreservice.Value("username", request.Username),
		coreservice.Value("password", request.Password),
	}
	fields = appendPresentUserFields(fields, map[string]bool{
		"departmentId": request.DepartmentID != nil,
		"name":         request.Name != nil,
		"nickName":     request.NickName != nil,
		"headImg":      request.HeadImg != nil,
		"phone":        request.Phone != nil,
		"email":        request.Email != nil,
		"remark":       request.Remark != nil,
		"status":       request.Status != nil,
	}, request.DepartmentID, request.Name, request.NickName, request.HeadImg, request.Phone, request.Email, request.Remark, request.Status)
	mutable, err := coreservice.NewMutable[entity.User, uint64](descriptor, fields)
	if err != nil {
		return coreservice.AddInput[entity.User]{}, err
	}

	return coreservice.NewAddObject[entity.User, uint64](descriptor, mutable)
}

func (request *UserUpdateReq) input(
	descriptor coreentity.Descriptor[entity.User, uint64],
) (coreservice.UpdateInput[entity.User, uint64], *[]uint64, error) {
	fields := make([]coreservice.FieldValue, 0, len(request.submitted))
	fields = appendPresentUserFields(fields, request.submitted, request.DepartmentID, request.Name, request.NickName, request.HeadImg, request.Phone, request.Email, request.Remark, request.Status)
	if request.submitted["username"] {
		fields = appendUserField(fields, "username", request.Username)
	}
	if request.submitted["password"] && request.Password != nil && strings.TrimSpace(*request.Password) != "" {
		fields = append(fields, coreservice.Value("password", *request.Password))
	}
	mutable, err := coreservice.NewMutable[entity.User, uint64](descriptor, fields)
	if err != nil {
		return coreservice.UpdateInput[entity.User, uint64]{}, nil, err
	}
	item, err := coreservice.NewUpdateItem[entity.User, uint64](descriptor, request.ID, mutable)
	if err != nil {
		return coreservice.UpdateInput[entity.User, uint64]{}, nil, err
	}
	input, err := coreservice.NewUpdateObject[entity.User, uint64](descriptor, item)
	if err != nil {
		return coreservice.UpdateInput[entity.User, uint64]{}, nil, err
	}
	var roleIDs *[]uint64
	if request.submitted["roleIdList"] {
		values := []uint64{}
		if request.RoleIDList != nil {
			values = append(values, (*request.RoleIDList)...)
		}
		roleIDs = &values
	}

	return input, roleIDs, nil
}

func appendPresentUserFields(
	fields []coreservice.FieldValue,
	submitted map[string]bool,
	departmentID *uint64,
	name *string,
	nickName *string,
	headImg *string,
	phone *string,
	email *string,
	remark *string,
	status *int32,
) []coreservice.FieldValue {
	values := []struct {
		name  string
		value any
	}{
		{"departmentId", departmentID},
		{"name", name},
		{"nickName", nickName},
		{"headImg", headImg},
		{"phone", phone},
		{"email", email},
		{"remark", remark},
		{"status", status},
	}
	for _, value := range values {
		if submitted[value.name] {
			fields = appendUserField(fields, value.name, value.value)
		}

	}

	return fields
}

func appendUserField(fields []coreservice.FieldValue, name string, value any) []coreservice.FieldValue {
	switch current := value.(type) {
	case *uint64:
		if current == nil {
			return append(fields, coreservice.Null(name))
		}
		return append(fields, coreservice.Value(name, *current))
	case *int32:
		if current == nil {
			return append(fields, coreservice.Null(name))
		}
		return append(fields, coreservice.Value(name, *current))
	case *string:
		if current == nil {
			return append(fields, coreservice.Null(name))
		}
		return append(fields, coreservice.Value(name, *current))
	default:
		return fields
	}
}

// UserController 声明系统用户管理路由。
func UserController(user *service.UserService, handler *UserHandler) controller.Definition {
	return controller.Admin("").
		Options(controller.RouterOptions{Description: "系统用户", TagName: "系统用户"}).
		Curd(controller.CurdOption{
			API:            controller.APIs(controller.APIDelete, controller.APIInfo, controller.APIList),
			Entity:         entity.User{},
			Service:        user,
			HiddenFields:   []controller.ColumnRef{controller.Field("password")},
			ReadonlyFields: []controller.ColumnRef{controller.Field("passwordV"), controller.Field("socketId")},
		}).
		Route(
			controller.Route{
				Method:     http.MethodPost,
				Path:       "/add",
				Summary:    "新增",
				Handler:    controller.Handle(handler.Add),
				Bind:       controller.BindJSON,
				Permission: "base:sys:user:add",
			},
			controller.Route{
				Method:     http.MethodPost,
				Path:       "/update",
				Summary:    "更新",
				Handler:    controller.Handle(handler.Update),
				Bind:       controller.BindJSON,
				Permission: "base:sys:user:update",
			},
			controller.Route{
				Method:      http.MethodPost,
				Path:        "/page",
				Summary:     "分页查询",
				Handler:     controller.Handle(handler.Page),
				Bind:        controller.BindJSON,
				Permission:  "base:sys:user:page",
				Transaction: controller.NonTransactional(),
			},
			controller.Route{
				Method:     http.MethodPost,
				Path:       "/move",
				Summary:    "移动部门",
				Handler:    controller.Handle(handler.Move),
				Bind:       controller.BindJSON,
				Permission: "base:sys:user:move",
			},
		).
		Build()
}
