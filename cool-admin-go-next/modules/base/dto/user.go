package dto

import (
	"bytes"
	"encoding/json"

	"github.com/gogf/gf/v2/os/gtime"
)

// 用户新增或更新时提交的角色关系
type UserRoleInput struct {
	RoleIDList []uint64 `json:"roleIdList"`
}

// 批量移动用户部门的请求
type UserMoveReq struct {
	DepartmentID uint64   `json:"departmentId" v:"required"`
	UserIDs      []uint64 `json:"userIds" v:"required"`
}

// 用户分页固定筛选请求
type UserPageReq struct {
	Page          int      `json:"page"`
	Size          int      `json:"size"`
	DepartmentIDs []uint64 `json:"departmentIds"`
	KeyWord       string   `json:"keyWord"`
	Status        *int32   `json:"status"`
}

// 新增用户及其角色关系的请求
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

// 更新用户及其角色关系的请求
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

// 严格解码并记录更新字段是否提交
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

// 报告请求 JSON 是否显式提交了该字段
func (request *UserUpdateReq) HasField(name string) bool {
	return request != nil && request.submitted[name]
}

// 当前用户可修改的个人资料白名单
type PersonUpdateReq struct {
	Name        *string `json:"name"`
	NickName    *string `json:"nickName"`
	HeadImg     *string `json:"headImg"`
	Phone       *string `json:"phone"`
	Email       *string `json:"email"`
	Password    *string `json:"password"`
	OldPassword *string `json:"oldPassword"`
}

// 用户分页列表的稳定响应字段
type UserPageItem struct {
	ID             uint64      `json:"id"`
	CreateTime     *gtime.Time `json:"createTime"`
	UpdateTime     *gtime.Time `json:"updateTime"`
	DepartmentID   *uint64     `json:"departmentId"`
	Name           *string     `json:"name"`
	Username       string      `json:"username"`
	NickName       *string     `json:"nickName"`
	HeadImg        *string     `json:"headImg"`
	Phone          *string     `json:"phone"`
	Email          *string     `json:"email"`
	Remark         *string     `json:"remark"`
	Status         int32       `json:"status"`
	DepartmentName *string     `json:"departmentName"`
	RoleIDs        []uint64    `json:"roleIds"`
	RoleName       string      `json:"roleName"`
}

// 用户详情表单的稳定响应字段
type UserInfoResult struct {
	ID             uint64      `json:"id"`
	CreateTime     *gtime.Time `json:"createTime"`
	UpdateTime     *gtime.Time `json:"updateTime"`
	DepartmentID   *uint64     `json:"departmentId"`
	UserID         *uint64     `json:"userId"`
	Name           *string     `json:"name"`
	Username       string      `json:"username"`
	PasswordV      int32       `json:"passwordV"`
	NickName       *string     `json:"nickName"`
	HeadImg        *string     `json:"headImg"`
	Phone          *string     `json:"phone"`
	Email          *string     `json:"email"`
	Remark         *string     `json:"remark"`
	Status         int32       `json:"status"`
	SocketID       *string     `json:"socketId"`
	DepartmentName *string     `json:"departmentName"`
	RoleIDList     []uint64    `json:"roleIdList"`
}
