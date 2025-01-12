package service

import (
	"errors"
	"gorm.io/gorm"
	"ops-api/dao"
	"ops-api/global"
	"ops-api/model"
)

var Group group

type group struct{}

// GroupCreate 创建构体，定义新增时的字段信息
type GroupCreate struct {
	Name        string `json:"name" binding:"required"`
	IsRoleGroup bool   `json:"is_role_group" default:"false"`
}

// GroupUpdate 更新分组名称构体
type GroupUpdate struct {
	ID   uint   `json:"id" binding:"required"`
	Name string `json:"name" binding:"required"`
}

// GroupUpdateUser 更新分组用户构体
type GroupUpdateUser struct {
	ID    uint   `json:"id" binding:"required"`
	Users []uint `json:"users" binding:"required"`
}

// GroupUpdatePermission 更新分组权限结构体
type GroupUpdatePermission struct {
	ID              uint     `json:"id" binding:"required"`
	MenuPermissions []string `json:"menu_permissions" binding:"required"`
	PathPermissions []string `json:"path_permissions" binding:"required"`
}

// GetGroupList 获取列表
func (u *group) GetGroupList(name string, page, limit int) (data *dao.GroupList, err error) {
	data, err = dao.Group.GetGroupList(name, page, limit)
	if err != nil {
		return nil, err
	}
	return data, nil
}

// AddGroup 创建分组
func (u *group) AddGroup(data *GroupCreate) (authGroup *model.AuthGroup, err error) {

	group := &model.AuthGroup{
		Name:        data.Name,
		IsRoleGroup: data.IsRoleGroup,
	}

	return dao.Group.AddGroup(group)
}

// DeleteGroup 删除分组
func (u *group) DeleteGroup(id int) (err error) {

	// 开启事务
	tx := global.MySQLClient.Begin()

	group := &model.AuthGroup{}
	if err := tx.First(group, id).Error; err != nil {
		tx.Rollback()
		return err
	}

	// 删除分组
	if err := dao.Group.DeleteGroup(tx, group); err != nil {
		tx.Rollback()
		return err
	}

	// 删除角色
	if err := dao.CasBin.DeleteRole(tx, group.Name); err != nil {
		tx.Rollback()
		return err
	}

	// 提交事务
	if err := tx.Commit().Error; err != nil {
		tx.Rollback()
		return err
	}

	// 重新加载策略
	if err := global.CasBinServer.LoadPolicy(); err != nil {
		return err
	}

	return nil
}

// UpdateGroup 更新
func (u *group) UpdateGroup(data *GroupUpdate) (*model.AuthGroup, error) {

	// 开启事务
	tx := global.MySQLClient.Begin()

	// 查询要修改的分组
	group := &model.AuthGroup{}
	if err := global.MySQLClient.First(group, data.ID).Error; err != nil {
		return nil, err
	}

	// 如果是角色用户组，同步更新名称到CasBin策略表
	if err := dao.CasBin.UpdateRoleName(tx, group.Name, data.Name); err != nil {
		return nil, err
	}

	// 更新分组名称
	group.Name = data.Name
	result, err := dao.Group.UpdateGroup(tx, group)
	if err != nil {
		return nil, err
	}

	// 提交事务
	if err := tx.Commit().Error; err != nil {
		tx.Rollback()
		return nil, err
	}

	// 重新加载策略
	if err := global.CasBinServer.LoadPolicy(); err != nil {
		return nil, err
	}

	return result, nil
}

// UpdateGroupPermission 更新组权限
func (u *group) UpdateGroupPermission(data *GroupUpdatePermission) (err error) {

	// 查询要修改的用户组
	group := &model.AuthGroup{}
	if err := global.MySQLClient.First(group, data.ID).Error; err != nil {
		return err
	}

	if !group.IsRoleGroup {
		return errors.New("普通分组不支持权限分配")
	}

	// 更新角色关联权限
	if err := dao.CasBin.UpdateRolePermission(group.Name, data.MenuPermissions, data.PathPermissions); err != nil {
		return err
	}

	// 重新加载策略
	if err := global.CasBinServer.LoadPolicy(); err != nil {
		return err
	}

	return nil
}

// UpdateGroupUser 更新组内用户，如果是角色用户组则支持同步用户信息到CasBin策略表
func (u *group) UpdateGroupUser(data *GroupUpdateUser) (*model.AuthGroup, error) {

	// 开启事务
	tx := global.MySQLClient.Begin()

	// 查询要修改的用户组
	group := &model.AuthGroup{}
	if err := tx.First(group, data.ID).Error; err != nil {
		tx.Rollback()
		return nil, err
	}

	// Users=0需要执行清空操作
	if len(data.Users) == 0 {
		return nil, errors.New("角色分组需要至少保留一个用户")
	}

	// 查询出要更新的所有用户
	var users []model.AuthUser
	if err := tx.Find(&users, data.Users).Error; err != nil {
		tx.Rollback()
		return nil, err
	}

	// 更新组内用户信息
	result, err := dao.Group.UpdateGroupUser(tx, group, users)
	if err != nil {
		tx.Rollback()
		return nil, err
	}

	// 同步角色用户组信息到CasBin策略表
	if group.IsRoleGroup {
		// 根据用户ID列表，将列表中的内容转换为用户名列表
		usernames, _ := GetUserNamesFromIDs(tx, data.Users)

		if err := dao.CasBin.UpdateRoleUser(tx, group.Name, usernames); err != nil {
			tx.Rollback()
			return nil, err
		}
	}

	// 提交事务
	if err := tx.Commit().Error; err != nil {
		tx.Rollback()
		return nil, err
	}

	// 重新加载策略
	if err := global.CasBinServer.LoadPolicy(); err != nil {
		return nil, err
	}

	return result, nil
}

// GetUserNamesFromIDs 根据用户ID列表返回对应的用户名列表
func GetUserNamesFromIDs(tx *gorm.DB, userIDs []uint) ([]string, error) {
	var usernames []string

	if err := tx.Model(&model.AuthUser{}).Select("username").Where("id IN (?)", userIDs).Find(&usernames).Error; err != nil {
		return nil, err
	}

	return usernames, nil
}
