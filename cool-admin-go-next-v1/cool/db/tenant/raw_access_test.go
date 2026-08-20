package tenant

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
)

var rawAccessOperations = map[string]struct{}{
	"Exec":     {},
	"GetAll":   {},
	"GetCount": {},
	"GetOne":   {},
	"Model":    {},
}

type rawAccessApproval struct {
	File        string
	Function    string
	Operation   string
	Fingerprint string
	Occurrence  int
	Purpose     string
}

type rawAccessCall struct {
	rawAccessApproval
	Source string
}

var approvedRawAccess = []rawAccessApproval{
	approveRawAccess("modules/base/service/sys/auth_boundary.go", "authorizationRolesFromDatabase", "Model", "6a5292bb76985b5eae6170bccd6011e30c942ea86235dd4c3c1b35772bafd81b", "读取候选角色及租户归属，随后统一校验是否可分配"),
	approveRawAccess("modules/base/service/sys/auth_boundary.go", "authorizationRolesFromDatabase", "Model", "d2ef301da746e6b4d4c16c7d8d84d8ca4fa724e0a7d866e6439de1cda9137f8a", "读取调用者已持有的候选角色，角色租户已由同函数校验"),
	approveRawAccess("modules/base/service/sys/auth_boundary.go", "authorizationUserFromDatabase", "Model", "12976e1286f2320b03104356a85f32e7dd944bb5bba0046c51a213afb1a98847", "读取授权用户状态与租户归属，供跨租户拒绝判断"),
	approveRawAccess("modules/base/service/sys/auth_boundary.go", "authorizationUserFromDatabase", "Model", "b351d4ef7eb0330b5a708b163c2759b08cfe9838213d9b7754e8f82ec1d7d8a0", "检测用户是否持有平台 admin 角色，角色查询显式限定平台租户"),
	approveRawAccess("modules/base/service/sys/auth_boundary.go", "isPlatformAdministrator", "GetCount", "a253ec19a4c9aea8d4a34aad6e9b19c240bebe9fce8dd088e6b1aa1b4fc6e3d9", "平台管理员判定，SQL 同时限定用户和 admin 角色为平台租户"),
	approveRawAccess("modules/base/service/sys/auth_boundary.go", "lockAuthorizationUsers", "GetAll", "8650a5504e989e8088aaabb4263c7221f9746d97865846efb2ae8f8babb9906b", "按排序 ID 锁定授权参与者，锁后读取租户归属并执行权限校验"),
	approveRawAccess("modules/base/service/sys/conf.go", "(*ConfService).GetValue", "Model", "b9f2faee83a08bfe35913d500400c74cb8cb606240b35f1d14184d084a90f994", "读取不含 tenant_id 的全局系统配置"),
	approveRawAccess("modules/base/service/sys/conf.go", "(*ConfService).UpdateValue", "Model", "b9f2faee83a08bfe35913d500400c74cb8cb606240b35f1d14184d084a90f994", "更新不含 tenant_id 的全局系统配置"),
	approveRawAccess("modules/base/service/sys/department.go", "(*DepartmentService).Delete", "Model", "03b9689b141a29b87cad7ec63e8fbe37033bd85aee342bbea27089de165f2389", "关联用户已按租户锁定，锁定并归档其无租户用户角色关系"),
	approveRawAccess("modules/base/service/sys/department.go", "(*DepartmentService).Delete", "Model", "03b9689b141a29b87cad7ec63e8fbe37033bd85aee342bbea27089de165f2389", "关联用户已按租户锁定，删除已归档用户角色关系", 2),
	approveRawAccess("modules/base/service/sys/department.go", "(*DepartmentService).Delete", "Model", "06f1f8d7d4c1ba63e727023f280a2a13d19ab39c28520b0077f7b6a00ca16dde", "部门用户已按当前租户验证，锁定完整用户快照用于归档"),
	approveRawAccess("modules/base/service/sys/department.go", "(*DepartmentService).Delete", "Model", "06f1f8d7d4c1ba63e727023f280a2a13d19ab39c28520b0077f7b6a00ca16dde", "部门用户已按当前租户验证，删除已归档用户", 2),
	approveRawAccess("modules/base/service/sys/department.go", "(*DepartmentService).Delete", "Model", "f576e91004ce57c82043eac6b4da5049d3d4181ebbad437ecf59d4069f2f9b14", "目标部门已在同事务按租户锁定，归档并删除其角色部门关系"),
	approveRawAccess("modules/base/service/sys/department.go", "(*DepartmentService).Delete", "Model", "d3405eaeb2194fc86b7905d75e5872fa108d57e23fc081a8882cdfdf33d5e2cd", "在事务内按租户锁定待删除部门"),
	approveRawAccess("modules/base/service/sys/department.go", "(*DepartmentService).Delete", "Model", "d3405eaeb2194fc86b7905d75e5872fa108d57e23fc081a8882cdfdf33d5e2cd", "查询当前租户的兜底顶级部门", 2),
	approveRawAccess("modules/base/service/sys/department.go", "(*DepartmentService).Delete", "Model", "d3405eaeb2194fc86b7905d75e5872fa108d57e23fc081a8882cdfdf33d5e2cd", "按租户删除已锁定部门", 3),
	approveRawAccess("modules/base/service/sys/department.go", "(*DepartmentService).Delete", "Model", "f09016db4f207b75755b5c420cbed67bbb05ce3bf4aa291b6587170ae2ec9ff2", "按目标部门及当前租户查询关联用户"),
	approveRawAccess("modules/base/service/sys/department.go", "(*DepartmentService).Delete", "Model", "f09016db4f207b75755b5c420cbed67bbb05ce3bf4aa291b6587170ae2ec9ff2", "按已验证用户 ID 将当前租户用户迁入兜底部门", 2),
	approveRawAccess("modules/base/service/sys/department.go", "(*DepartmentService).List", "GetAll", "9f3f7470fa126f81174c6fe078b2fad14484fba420452c0b2cf4890b60036156", "部门列表同时限制主部门与父部门租户，平台作用域允许全局管理"),
	approveRawAccess("modules/base/service/sys/department.go", "departmentIDsForUser", "GetAll", "c0c23cfb1355aec61d29469495f73e5e70c9c64810711074946e2bb8459352ab", "联查用户角色部门关系，租户作用域下同时限定用户、角色和部门"),
	approveRawAccess("modules/base/service/sys/log.go", "(*LogService).Page", "GetAll", "7be69d148f636479c599597eba5ad10ed5d11700833f05f0740f6df46800fcad", "日志分页查询对日志主表和用户联表应用编译租户条件"),
	approveRawAccess("modules/base/service/sys/log.go", "(*LogService).Page", "GetCount", "cffa5ebcd43b23014ecfac98111b265c91588861340ee07aaf75257ebeca83b9", "日志分页计数与列表复用相同的主表及联表租户条件"),
	approveRawAccess("modules/base/service/sys/login.go", "(*AuthService).userByUsername", "GetOne", "8b9d873f38d31250389bb9387dd6b1a683a3e8137c9d695f975ebabfe88cc372", "登录前尚无可信租户上下文，按用户名读取用户并从数据库生成租户身份"),
	approveRawAccess("modules/base/service/sys/login.go", "roleIDsByUserIDFromTX", "GetAll", "d0bf4729d45a66b512c8e41c5471650a4337ca9354befaa85a44101f95e95e2d", "用户已在同事务锁定并验证，读取服务端维护的角色关系生成声明"),
	approveRawAccess("modules/base/service/sys/login.go", "userByIDFromTX", "GetOne", "7d8e8fb45ced222e478200866a2d764ac6e3cdb687502cf2333c97cd3ba339d3", "刷新令牌事务内按已签名用户 ID 锁定并重建认证快照"),
	approveRawAccess("modules/base/service/sys/menu.go", "(*MenuService).Delete", "Model", "0e23e1f47588c3a16595498e95afd0a05d53360796c15ca70e766cf92992ac61", "菜单及子菜单已在同事务按租户锁定，归档并删除对应角色菜单关系"),
	approveRawAccess("modules/base/service/sys/menu.go", "(*MenuService).Export", "GetAll", "e953c4e1f72a01b502f73c4df36785e2f5b55e31a83b674634090fbbb4b0fc6b", "导出查询按编译租户条件限制选中菜单"),
	approveRawAccess("modules/base/service/sys/menu.go", "(*MenuService).Info", "GetOne", "57d7b229ff051f5dc7c4eb042fd03947462459045e7c65ecfd7161ea01b74bd4", "菜单详情按编译租户条件和 ID 查询"),
	approveRawAccess("modules/base/service/sys/menu.go", "(*MenuService).List", "GetAll", "25af48fad60f73778a435d31121394226550d557f8f05cbc30f13759fab906bc", "菜单列表分别限制菜单、父菜单和非超管角色租户"),
	approveRawAccess("modules/base/service/sys/menu.go", "(*MenuService).Page", "GetAll", "c7a661f21bcc2816e93664de6294d394a640d9db63ff6dfd2a1fbcdea0966a79", "菜单分页列表使用与计数一致的编译租户条件"),
	approveRawAccess("modules/base/service/sys/menu.go", "(*MenuService).Page", "GetCount", "7909727360b9fbf14074e022d67bdc538081b99200435ff18a79ddfe1f8b28e6", "菜单分页计数按编译租户条件查询"),
	approveRawAccess("modules/base/service/sys/param.go", "(*ParamService).Info", "GetOne", "d6ee57655794ef11333c2d576d0465d8aa15771cf108f4b5ebec292b101892dc", "参数详情按编译租户条件和 ID 查询"),
	approveRawAccess("modules/base/service/sys/param.go", "(*ParamService).Page", "GetAll", "bdd1f9678cf03af32626769d79eeb492233ce6269fe3c5d57f9c422b523b5fc3", "参数分页列表使用与计数一致的编译租户条件"),
	approveRawAccess("modules/base/service/sys/param.go", "(*ParamService).Page", "GetCount", "2b3b036828b09d6ac1a53d44295d9a2fb71b13dbf99f1c17247d38a89d7080c9", "参数分页计数按编译租户条件查询"),
	approveRawAccess("modules/base/service/sys/param.go", "(*ParamService).publicParamModel", "Model", "b9f2faee83a08bfe35913d500400c74cb8cb606240b35f1d14184d084a90f994", "公开参数在缺失或平台上下文显式应用 GlobalOnly 谓词"),
	approveRawAccess("modules/base/service/sys/perms.go", "(*PermissionService).allMenuRows", "GetAll", "faba276c6b422d22c20480c390f32f49ad05bfb9da873e3b562b4d022f648f25", "平台管理员读取菜单；派生租户作用域仍显式限制菜单租户"),
	approveRawAccess("modules/base/service/sys/perms.go", "(*PermissionService).userMenuRows", "GetAll", "b8e17e28d316aaaa982bc8d396b40963f738495b564dcdfeb65c087775b96a26", "联查当前用户、角色和菜单，租户作用域同时限制三类主资源"),
	approveRawAccess("modules/base/service/sys/perms.go", "(*PermissionService).userMenuRows", "GetAll", "e3db1b8a9d44dde6e6cbcdd951ffdb1c6614b39d7fc110bd12ab38a3711aa55c", "按菜单租户读取父链索引，最终仅返回已授权菜单及同租户父级"),
	approveRawAccess("modules/base/service/sys/role.go", "(*RoleService).Add", "Model", "d3405eaeb2194fc86b7905d75e5872fa108d57e23fc081a8882cdfdf33d5e2cd", "事务内校验菜单部门归属后写入服务端租户角色"),
	approveRawAccess("modules/base/service/sys/role.go", "(*RoleService).Delete", "Model", "d3405eaeb2194fc86b7905d75e5872fa108d57e23fc081a8882cdfdf33d5e2cd", "事务内按当前租户锁定并校验待删除角色"),
	approveRawAccess("modules/base/service/sys/role.go", "(*RoleService).Delete", "Model", "d3405eaeb2194fc86b7905d75e5872fa108d57e23fc081a8882cdfdf33d5e2cd", "按已锁定角色 ID 和当前租户删除角色", 2),
	approveRawAccess("modules/base/service/sys/role.go", "(*RoleService).Delete", "Model", "69e3e39816b4074d421238833a9b90823a12780e6747ed6858ef40389e597d38", "角色已按租户锁定，归档并删除固定集合中的无租户关系表记录"),
	approveRawAccess("modules/base/service/sys/role.go", "(*RoleService).Info", "GetAll", "5b23785cea2ae0f7700fbb777d335f1a7cb890b058b69612e29991a05d8fa476", "角色详情联查部门关系，普通租户显式限制部门租户"),
	approveRawAccess("modules/base/service/sys/role.go", "(*RoleService).Info", "GetAll", "ce46e6aff902fb322df83d9b30c092dc121317036897b7ada269089a74a8655a", "角色详情联查菜单关系，普通租户显式限制菜单租户"),
	approveRawAccess("modules/base/service/sys/role.go", "(*RoleService).Info", "GetOne", "59900df2e9e645f1434247cb5f9bc414cf6d3e6607a7cbc4a78fe818ee693da2", "角色详情主记录按 ID 和当前租户查询"),
	approveRawAccess("modules/base/service/sys/role.go", "(*RoleService).List", "GetAll", "ad0e0f594b030b67bdf3f50b4879571f630e21f25132993f0ec36ecbcccd5d0f", "角色列表使用统一 roleWhere 租户条件"),
	approveRawAccess("modules/base/service/sys/role.go", "(*RoleService).Page", "GetAll", "124bdc2803146979fce2270829367211ea9fda272f90d80d75cef75f4a316458", "角色分页列表使用与计数一致的 roleWhere 租户条件"),
	approveRawAccess("modules/base/service/sys/role.go", "(*RoleService).Page", "GetCount", "ded2b44b1115c3792bb031b8d1962a6fbf87d6320810ed78bc4ffd3add56b5f4", "角色分页计数使用统一 roleWhere 租户条件"),
	approveRawAccess("modules/base/service/sys/role.go", "(*RoleService).Update", "Model", "d3405eaeb2194fc86b7905d75e5872fa108d57e23fc081a8882cdfdf33d5e2cd", "事务内按当前租户锁定角色并读取旧关系"),
	approveRawAccess("modules/base/service/sys/role.go", "(*RoleService).Update", "Model", "d3405eaeb2194fc86b7905d75e5872fa108d57e23fc081a8882cdfdf33d5e2cd", "资源关系归属验证后按 ID 和租户更新角色", 2),
	approveRawAccess("modules/base/service/sys/role.go", "(*RoleService).ensureLabelUnique", "Model", "b9f2faee83a08bfe35913d500400c74cb8cb606240b35f1d14184d084a90f994", "角色标签唯一性检查显式限制当前租户"),
	approveRawAccess("modules/base/service/sys/role.go", "(*RoleService).validateRoleRelations", "Model", "2448bdcd35148afeb4e69b1e0f090f6370a69d88d4e1ccd8567c1403dcb70fb8", "事务内锁定固定菜单或部门表的候选资源并校验租户归属"),
	approveRawAccess("modules/base/service/sys/role.go", "relationIDs", "Model", "e3de6f46d2044456fef6afec746aeac76d3a6e4921b35386fb1183fa7c337e29", "读取已按租户锁定角色在固定关系表中的当前关系"),
	approveRawAccess("modules/base/service/sys/role.go", "replaceRoleRelations", "Model", "389c3cb155493cde58ddcf0fdefdc0f2a29fc1e26880da5dbfe674a05a022ffa", "角色及部门已在同事务完成租户验证，删除旧角色部门关系"),
	approveRawAccess("modules/base/service/sys/role.go", "replaceRoleRelations", "Model", "389c3cb155493cde58ddcf0fdefdc0f2a29fc1e26880da5dbfe674a05a022ffa", "角色及部门已在同事务完成租户验证，写入新角色部门关系", 2),
	approveRawAccess("modules/base/service/sys/role.go", "replaceRoleRelations", "Model", "4c683a91abe80acdbd6b1d97ffea80a1b3aef2decf9f31919afdf0d20aed93b2", "角色及菜单已在同事务完成租户验证，删除旧角色菜单关系"),
	approveRawAccess("modules/base/service/sys/role.go", "replaceRoleRelations", "Model", "4c683a91abe80acdbd6b1d97ffea80a1b3aef2decf9f31919afdf0d20aed93b2", "角色及菜单已在同事务完成租户验证，写入新角色菜单关系", 2),
	approveRawAccess("modules/base/service/sys/role.go", "roleUserIDsForScope", "Model", "707da8f5cbad674704f5637928819dae484ac6036722a8bb2bfb929f6e06f06d", "读取已验证角色关联用户，普通租户同时限制用户租户"),
	approveRawAccess("modules/base/service/sys/user.go", "(*UserService).Add", "Model", "b9f2faee83a08bfe35913d500400c74cb8cb606240b35f1d14184d084a90f994", "新增用户前按当前租户检查用户名唯一性"),
	approveRawAccess("modules/base/service/sys/user.go", "(*UserService).Add", "Model", "d3405eaeb2194fc86b7905d75e5872fa108d57e23fc081a8882cdfdf33d5e2cd", "授权和部门校验通过后写入服务端租户用户"),
	approveRawAccess("modules/base/service/sys/user.go", "(*UserService).Delete", "Model", "03b9689b141a29b87cad7ec63e8fbe37033bd85aee342bbea27089de165f2389", "用户已在同事务按租户锁定，归档并删除其用户角色关系"),
	approveRawAccess("modules/base/service/sys/user.go", "(*UserService).Delete", "Model", "d3405eaeb2194fc86b7905d75e5872fa108d57e23fc081a8882cdfdf33d5e2cd", "按已锁定用户 ID 和当前租户删除用户"),
	approveRawAccess("modules/base/service/sys/user.go", "(*UserService).Info", "GetAll", "3cd2fba533e648dd93039f1e82e959d1a6755759b0fa15b5bacf82d7038812d3", "用户详情主记录已按租户验证，角色联查同时限制角色租户"),
	approveRawAccess("modules/base/service/sys/user.go", "(*UserService).Info", "GetOne", "4d82e7d71f097b06659f33996e29b54599ecec629c0519e5cb99994aa862f51e", "用户详情限制用户主表租户，部门联表同时限制部门租户"),
	approveRawAccess("modules/base/service/sys/user.go", "(*UserService).Move", "Model", "29a2a11c3580865f0c73b33737c3734147e37ba0e69c9fd29bc37a1c384bf1b8", "事务内按当前租户锁定目标部门"),
	approveRawAccess("modules/base/service/sys/user.go", "(*UserService).Move", "Model", "d3405eaeb2194fc86b7905d75e5872fa108d57e23fc081a8882cdfdf33d5e2cd", "事务内按当前租户排序锁定待移动用户集合"),
	approveRawAccess("modules/base/service/sys/user.go", "(*UserService).Move", "Model", "d3405eaeb2194fc86b7905d75e5872fa108d57e23fc081a8882cdfdf33d5e2cd", "部门和用户均锁定后按用户 ID 与租户更新部门", 2),
	approveRawAccess("modules/base/service/sys/user.go", "(*UserService).Page", "GetAll", "3cd2fba533e648dd93039f1e82e959d1a6755759b0fa15b5bacf82d7038812d3", "分页主记录已按租户筛选，角色联查同时限制角色租户"),
	approveRawAccess("modules/base/service/sys/user.go", "(*UserService).Page", "GetAll", "b6d59cc1607cb9b89768c6ed499ab9a8b80954bd9265257fc03e9fc0c2fc737b", "用户分页限制用户主表租户，部门联表同时限制部门租户"),
	approveRawAccess("modules/base/service/sys/user.go", "(*UserService).Page", "GetCount", "cecdf66a9f9314f5954d9f00db738bb0185c89d8f7d190283a716e2b9648daae", "用户分页计数按主表别名租户条件查询"),
	approveRawAccess("modules/base/service/sys/user.go", "(*UserService).Person", "GetOne", "b4c46c00153e26bad2656ea7828d2ad740ae215c652ac9f6d94f39e90b8562ec", "按认证上下文中的当前用户 ID 读取个人资料"),
	approveRawAccess("modules/base/service/sys/user.go", "(*UserService).PersonUpdate", "Model", "d3405eaeb2194fc86b7905d75e5872fa108d57e23fc081a8882cdfdf33d5e2cd", "事务内按认证用户 ID 锁定并校验个人资料"),
	approveRawAccess("modules/base/service/sys/user.go", "(*UserService).PersonUpdate", "Model", "d3405eaeb2194fc86b7905d75e5872fa108d57e23fc081a8882cdfdf33d5e2cd", "按认证用户 ID 更新个人资料并原子递增密码版本", 2),
	approveRawAccess("modules/base/service/sys/user.go", "(*UserService).Update", "Model", "b9f2faee83a08bfe35913d500400c74cb8cb606240b35f1d14184d084a90f994", "更新用户前按当前租户检查用户名唯一性"),
	approveRawAccess("modules/base/service/sys/user.go", "(*UserService).Update", "Model", "d3405eaeb2194fc86b7905d75e5872fa108d57e23fc081a8882cdfdf33d5e2cd", "事务内按当前租户锁定目标用户并执行授权校验"),
	approveRawAccess("modules/base/service/sys/user.go", "(*UserService).Update", "Model", "d3405eaeb2194fc86b7905d75e5872fa108d57e23fc081a8882cdfdf33d5e2cd", "授权、部门和角色校验通过后按租户更新用户", 2),
	approveRawAccess("modules/base/service/sys/user.go", "(*UserService).validateUserDepartment", "Model", "6d0233f7656d6f8a4321785105ce4c121a742bd34429a2c67975913df8f87483", "用户写入前按当前租户验证部门归属"),
	approveRawAccess("modules/base/service/sys/user.go", "replaceUserRoles", "Model", "7efff7afaf80ca93f884da50a81fb48304b4ad47bca0448835312ed39068a0a7", "用户和角色已完成租户授权，删除旧用户角色关系"),
	approveRawAccess("modules/base/service/sys/user.go", "replaceUserRoles", "Model", "7efff7afaf80ca93f884da50a81fb48304b4ad47bca0448835312ed39068a0a7", "用户和角色已完成租户授权，写入新用户角色关系", 2),
	approveRawAccess("modules/dict/service/dict_info.go", "(*DictInfoService).GlobalTypes", "Model", "b29cd7c392d0ba117f9cb448c04f0eccbcf21bb2beab4a7531b52fa966eabcb8", "公开字典类型只读入口，后续显式应用 GlobalOnly 谓词"),
}

// approveRawAccess 创建默认序号为一的审计条目。
func approveRawAccess(file string, function string, operation string, fingerprint string, purpose string, occurrence ...int) rawAccessApproval {
	approvedOccurrence := 1
	if len(occurrence) > 0 {
		approvedOccurrence = occurrence[0]
	}
	return rawAccessApproval{
		File:        file,
		Function:    function,
		Operation:   operation,
		Fingerprint: fingerprint,
		Occurrence:  approvedOccurrence,
		Purpose:     purpose,
	}
}

// TestRawDatabaseAccessIsAudited 确保模块服务的直接数据库入口均经过逐项审计。
func TestRawDatabaseAccessIsAudited(t *testing.T) {
	repositoryRoot := rawAccessRepositoryRoot(t)
	calls := scanRawAccessCalls(t, repositoryRoot)
	approved := make(map[string]rawAccessApproval, len(approvedRawAccess))
	for _, approval := range approvedRawAccess {
		if strings.TrimSpace(approval.Purpose) == "" {
			t.Errorf("raw-access allowlist 用途为空: %s", approval.key())
			continue
		}
		key := approval.key()
		if _, exists := approved[key]; exists {
			t.Errorf("raw-access allowlist 重复: %s", key)
			continue
		}
		approved[key] = approval
	}

	actual := make(map[string]rawAccessCall, len(calls))
	for _, call := range calls {
		key := call.key()
		actual[key] = call
		if _, exists := approved[key]; !exists {
			t.Errorf("发现未审计的直接数据库入口: %s\n  source: %s", key, call.Source)
		}
	}
	for key, approval := range approved {
		if _, exists := actual[key]; !exists {
			t.Errorf("raw-access allowlist 条目已失效，请删除或重新审计: %s\n  purpose: %s", key, approval.Purpose)
		}
	}
}

// TestRawAccessScannerTracksDuplicateCalls 验证重复入口计数和安全辅助函数排除规则。
func TestRawAccessScannerTracksDuplicateCalls(t *testing.T) {
	fileSet := token.NewFileSet()
	parsed, err := parser.ParseFile(fileSet, "scanner_fixture.go", `package fixture
func (s *Service) Query() {
	s.DB.GetOne(ctx, "SELECT 1")
	s.DB.GetOne(ctx, "SELECT 1")
	tenant.ScopedModel(ctx, s.DB, definition, "")
}`, 0)
	if err != nil {
		t.Fatalf("解析扫描样例失败: %v", err)
	}
	declaration, ok := parsed.Decls[0].(*ast.FuncDecl)
	if !ok {
		t.Fatal("扫描样例缺少函数声明")
	}
	calls := rawAccessCallsInNode(fileSet, declaration.Body, "fixture.go", rawAccessFunctionName(fileSet, declaration))
	if len(calls) != 2 {
		t.Fatalf("直接数据库入口数量错误: got %d, want 2", len(calls))
	}
	if calls[0].Fingerprint != calls[1].Fingerprint {
		t.Fatal("相同调用应生成相同指纹")
	}
	if calls[0].Occurrence != 1 || calls[1].Occurrence != 2 {
		t.Fatalf("重复调用序号错误: got %d, %d", calls[0].Occurrence, calls[1].Occurrence)
	}
}

// rawAccessRepositoryRoot 返回当前测试文件所属仓库根目录。
func rawAccessRepositoryRoot(t *testing.T) string {
	t.Helper()
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("无法定位 raw-access 测试文件")
	}
	start := filepath.Dir(filename)
	for current := start; current != string(filepath.Separator); current = filepath.Dir(current) {
		if _, err := os.Stat(filepath.Join(current, "go.mod")); err == nil {
			return filepath.Clean(current)
		}
	}
	t.Fatalf("raw-access 测试未找到仓库根: %s", filename)
	return ""
}

// scanRawAccessCalls 扫描模块服务中的直接数据库调用。
func scanRawAccessCalls(t *testing.T, repositoryRoot string) []rawAccessCall {
	t.Helper()
	modulesRoot := filepath.Join(repositoryRoot, "modules")
	calls := make([]rawAccessCall, 0)
	err := filepath.WalkDir(modulesRoot, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if entry.Name() == "testdata" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			return nil
		}
		relativePath, err := filepath.Rel(repositoryRoot, path)
		if err != nil {
			return err
		}
		slashPath := filepath.ToSlash(relativePath)
		parts := strings.Split(slashPath, "/")
		if len(parts) < 4 || parts[0] != "modules" || parts[2] != "service" {
			return nil
		}
		fileCalls, err := scanRawAccessFile(path, slashPath)
		if err != nil {
			return err
		}
		calls = append(calls, fileCalls...)
		return nil
	})
	if err != nil {
		t.Fatalf("扫描模块服务失败: %v", err)
	}
	sort.Slice(calls, func(left int, right int) bool {
		return calls[left].key() < calls[right].key()
	})
	return calls
}

// scanRawAccessFile 解析单个服务文件并提取受控调用。
func scanRawAccessFile(filename string, relativePath string) ([]rawAccessCall, error) {
	fileSet := token.NewFileSet()
	parsed, err := parser.ParseFile(fileSet, filename, nil, 0)
	if err != nil {
		return nil, fmt.Errorf("解析 %s: %w", relativePath, err)
	}
	calls := make([]rawAccessCall, 0)
	for _, declaration := range parsed.Decls {
		switch current := declaration.(type) {
		case *ast.FuncDecl:
			calls = append(calls, rawAccessCallsInNode(fileSet, current.Body, relativePath, rawAccessFunctionName(fileSet, current))...)
		case *ast.GenDecl:
			calls = append(calls, rawAccessCallsInNode(fileSet, current, relativePath, "<package>")...)
		}
	}
	return calls, nil
}

// rawAccessCallsInNode 提取语法节点中的直接数据库调用。
func rawAccessCallsInNode(fileSet *token.FileSet, node ast.Node, relativePath string, function string) []rawAccessCall {
	if node == nil {
		return nil
	}
	calls := make([]rawAccessCall, 0)
	occurrences := make(map[string]int)
	ast.Inspect(node, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		selector, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		operation := selector.Sel.Name
		if _, tracked := rawAccessOperations[operation]; !tracked {
			return true
		}
		source, err := formatRawAccessNode(fileSet, call)
		if err != nil {
			source = operation
		}
		sum := sha256.Sum256([]byte(source))
		fingerprint := fmt.Sprintf("%x", sum)
		occurrenceKey := operation + "|" + fingerprint
		occurrences[occurrenceKey]++
		calls = append(calls, rawAccessCall{
			rawAccessApproval: rawAccessApproval{
				File:        relativePath,
				Function:    function,
				Operation:   operation,
				Fingerprint: fingerprint,
				Occurrence:  occurrences[occurrenceKey],
			},
			Source: compactRawAccessSource(source),
		})
		return true
	})
	return calls
}

// rawAccessFunctionName 返回包含接收者类型的方法名。
func rawAccessFunctionName(fileSet *token.FileSet, declaration *ast.FuncDecl) string {
	if declaration.Recv == nil || len(declaration.Recv.List) == 0 {
		return declaration.Name.Name
	}
	receiver, err := formatRawAccessNode(fileSet, declaration.Recv.List[0].Type)
	if err != nil {
		return declaration.Name.Name
	}
	return fmt.Sprintf("(%s).%s", receiver, declaration.Name.Name)
}

// formatRawAccessNode 返回稳定的格式化语法文本。
func formatRawAccessNode(fileSet *token.FileSet, node ast.Node) (string, error) {
	var buffer bytes.Buffer
	if err := format.Node(&buffer, fileSet, node); err != nil {
		return "", err
	}
	return buffer.String(), nil
}

// compactRawAccessSource 压缩调用文本，便于测试失败时审查。
func compactRawAccessSource(source string) string {
	compact := strings.Join(strings.Fields(source), " ")
	const maximumLength = 240
	if len(compact) <= maximumLength {
		return compact
	}
	return compact[:maximumLength] + "..."
}

// key 返回 allowlist 的稳定匹配键。
func (approval rawAccessApproval) key() string {
	return fmt.Sprintf("%s | %s | %s | %s | #%d", approval.File, approval.Function, approval.Operation, approval.Fingerprint, approval.Occurrence)
}
