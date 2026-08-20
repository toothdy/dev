package dto

import (
   "encoding/json"

   "github.com/toothdy/cool-admin-go-next/cool/exception"
)

// 个人信息更新请求
type PersonUpdateRequest struct {
   NickName    string `json:"nickName"`
   HeadImg     string `json:"headImg"`
   Phone       string `json:"phone"`
   Email       string `json:"email"`
   Remark      string `json:"remark"`
   OldPassword string `json:"oldPassword"`
   Password    string `json:"password"`
   present     map[string]bool
}

/**
 * 解析个人信息更新请求并记录字段是否出现
 * @param data JSON 请求数据
 * @returns 解析错误
 */
func (r *PersonUpdateRequest) UnmarshalJSON(data []byte) error {
   type plainPersonUpdateRequest PersonUpdateRequest
   var decoded plainPersonUpdateRequest
   if err := json.Unmarshal(data, &decoded); err != nil {
      return err
   }
   var fields map[string]json.RawMessage
   if err := json.Unmarshal(data, &fields); err != nil {
      return err
   }
   *r = PersonUpdateRequest(decoded)
   r.present = map[string]bool{}
   for field := range fields {
      switch field {
      case "nickName", "headImg", "phone", "email", "remark", "oldPassword", "password":
         r.present[field] = true
      }
   }
   return nil
}

// 返回经过校验的改密参数
func (r PersonUpdateRequest) PasswordChange() (oldPassword string, password string, changed bool, err error) {
   if r.present != nil && !r.present["password"] {
      return "", "", false, nil
   }
   password = r.Password
   if password == "" {
      return "", "", false, nil
   }
   oldPassword = r.OldPassword
   if oldPassword == "" {
      return "", "", false, exception.Validate("原密码不能为空")
   }
   return oldPassword, password, true, nil
}

/**
 * 转换为个人信息更新字段
 * @returns 允许更新的数据库字段
 */
func (r PersonUpdateRequest) Values() map[string]interface{} {
   values := make(map[string]interface{})
   if r.present == nil {
      values["nickName"] = r.NickName
      values["headImg"] = r.HeadImg
      values["phone"] = r.Phone
      values["email"] = r.Email
      values["remark"] = r.Remark
      return values
   }
   if r.present["nickName"] {
      values["nickName"] = r.NickName
   }
   if r.present["headImg"] {
      values["headImg"] = r.HeadImg
   }
   if r.present["phone"] {
      values["phone"] = r.Phone
   }
   if r.present["email"] {
      values["email"] = r.Email
   }
   if r.present["remark"] {
      values["remark"] = r.Remark
   }
   return values
}

// 用户移动部门请求
type MoveReq struct {
   DepartmentID int64   `json:"departmentId" v:"required#部门参数错误"`
   UserIDs      []int64 `json:"userIds" v:"required|length:1,500#用户不能为空|用户不能为空"`
}

/**
 * 校验用户移动部门请求
 * @returns 校验错误
 */
func (r MoveReq) Validate() error {
   userIDs := make(map[int64]struct{}, len(r.UserIDs))
   for _, userID := range r.UserIDs {
      if userID <= 0 {
         return exception.Validate("用户参数错误")
      }
      if _, exists := userIDs[userID]; exists {
         return exception.Validate("用户参数错误")
      }
      userIDs[userID] = struct{}{}
   }
   return nil
}