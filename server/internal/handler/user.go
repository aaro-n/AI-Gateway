package handler

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"

	"ai-gateway/internal/middleware"
	"ai-gateway/internal/model"
)

type UserHandler struct{}

func NewUserHandler() *UserHandler {
	return &UserHandler{}
}

// ── 请求/响应类型 ──

type createUserRequest struct {
	Username    string `json:"username" binding:"required,min=3,max=64"`
	Password    string `json:"password" binding:"required,min=6"`
	DisplayName string `json:"display_name"`
}

type updateUserRequest struct {
	Username    *string `json:"username"`
	Password    *string `json:"password"`
	DisplayName *string `json:"display_name"`
	Enabled     *bool   `json:"enabled"`
}

type updateUserPermissionsRequest struct {
	ProviderIDs []uint `json:"provider_ids"`
	ModelIDs    []uint `json:"model_ids"`
}

type userResponse struct {
	ID          uint   `json:"id"`
	Username    string `json:"username"`
	DisplayName string `json:"display_name"`
	Email       string `json:"email"`
	Role        string `json:"role"`
	Enabled     bool   `json:"enabled"`
	CreatedAt   string `json:"created_at"`
}

type userPermissionResponse struct {
	Providers []providerBrief `json:"providers"`
	Models    []modelBrief    `json:"models"`
}

type providerBrief struct {
	ID   uint   `json:"id"`
	Name string `json:"name"`
	Slug string `json:"slug"`
}

type modelBrief struct {
	ID   uint   `json:"id"`
	Name string `json:"name"`
	Slug string `json:"slug"`
}

// ── 用户 CRUD ──

// ListUsers 列出所有用户（admin 专用）。
func (h *UserHandler) ListUsers(c *gin.Context) {
	var users []model.User
	if err := model.DB.Order("id ASC").Find(&users).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	result := make([]userResponse, len(users))
	for i, u := range users {
		result[i] = userResponse{
			ID:          u.ID,
			Username:    u.Username,
			DisplayName: u.DisplayName,
			Email:       u.Email,
			Role:        u.Role,
			Enabled:     u.Enabled,
			CreatedAt:   u.CreatedAt.Format("2006-01-02 15:04:05"),
		}
	}

	c.JSON(http.StatusOK, gin.H{"users": result})
}

// CreateUser 创建普通用户（admin 专用）。
func (h *UserHandler) CreateUser(c *gin.Context) {
	var req createUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 检查用户名是否已存在
	var existing model.User
	if err := model.DB.Where("username = ?", req.Username).First(&existing).Error; err == nil {
		c.JSON(http.StatusConflict, gin.H{"error": "username already exists"})
		return
	}

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to hash password"})
		return
	}

	user := model.User{
		Username:     req.Username,
		PasswordHash: string(hashedPassword),
		DisplayName:  req.DisplayName,
		Role:         "user",
		Enabled:      true,
	}

	if err := model.DB.Create(&user).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"user": userResponse{
			ID:          user.ID,
			Username:    user.Username,
			DisplayName: user.DisplayName,
			Email:       user.Email,
			Role:        user.Role,
			Enabled:     user.Enabled,
			CreatedAt:   user.CreatedAt.Format("2006-01-02 15:04:05"),
		},
	})
}

// UpdateUser 更新用户信息（admin 专用）。
func (h *UserHandler) UpdateUser(c *gin.Context) {
	id, err := middleware.GetID(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	var user model.User
	if err := model.DB.First(&user, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
		return
	}

	// 不允许修改 admin 的角色
	if user.Role == "admin" {
		c.JSON(http.StatusForbidden, gin.H{"error": "cannot modify admin user"})
		return
	}

	var req updateUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	updates := map[string]interface{}{}

	if req.Username != nil {
		// 检查新用户名是否重复
		var dup model.User
		if err := model.DB.Where("username = ? AND id != ?", *req.Username, id).First(&dup).Error; err == nil {
			c.JSON(http.StatusConflict, gin.H{"error": "username already exists"})
			return
		}
		updates["username"] = *req.Username
	}
	if req.Password != nil {
		hashed, err := bcrypt.GenerateFromPassword([]byte(*req.Password), bcrypt.DefaultCost)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to hash password"})
			return
		}
		updates["password_hash"] = string(hashed)
	}
	if req.DisplayName != nil {
		updates["display_name"] = *req.DisplayName
	}
	if req.Enabled != nil {
		updates["enabled"] = *req.Enabled
	}

	if len(updates) > 0 {
		if err := model.DB.Model(&user).Updates(updates).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	}

	c.JSON(http.StatusOK, gin.H{"message": "user updated"})
}

// DeleteUser 删除用户（admin 专用，不能删除 admin 自己）。
func (h *UserHandler) DeleteUser(c *gin.Context) {
	id, err := middleware.GetID(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	var user model.User
	if err := model.DB.First(&user, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
		return
	}

	if user.Role == "admin" {
		c.JSON(http.StatusForbidden, gin.H{"error": "cannot delete admin user"})
		return
	}

	// 清理关联数据
	model.DB.Where("user_id = ?", id).Delete(&model.UserProvider{})
	model.DB.Where("user_id = ?", id).Delete(&model.UserModel{})

	// 清理该用户创建的 API 密钥
	var userKeys []model.Key
	model.DB.Where("user_id = ?", id).Find(&userKeys)
	for _, k := range userKeys {
		model.DB.Where("key_id = ?", k.ID).Delete(&model.KeyModel{})
		model.DB.Where("key_id = ?", k.ID).Delete(&model.KeyProvider{})
		model.DB.Where("key_id = ?", k.ID).Delete(&model.KeyProviderModel{})
		model.DB.Where("key_id = ?", k.ID).Delete(&model.KeyMCPTool{})
		model.DB.Where("key_id = ?", k.ID).Delete(&model.KeyMCPResource{})
		model.DB.Where("key_id = ?", k.ID).Delete(&model.KeyMCPPrompt{})
		model.DB.Where("key_id = ?", k.ID).Delete(&model.KeyFormat{})
	}
	model.DB.Where("user_id = ?", id).Delete(&model.Key{})

	// 软删除用户
	if err := model.DB.Delete(&user).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "user deleted"})
}

// ── 用户权限管理 ──

// GetUserPermissions 获取用户的访问权限（admin 专用）。
func (h *UserHandler) GetUserPermissions(c *gin.Context) {
	id, err := middleware.GetID(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	var user model.User
	if err := model.DB.First(&user, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
		return
	}

	// 获取授权的厂商
	var ups []model.UserProvider
	model.DB.Preload("Provider").Where("user_id = ?", id).Find(&ups)
	providers := make([]providerBrief, len(ups))
	for i, up := range ups {
		if up.Provider != nil {
			providers[i] = providerBrief{ID: up.ProviderID, Name: up.Provider.Name, Slug: up.Provider.Slug}
		}
	}

	// 获取授权的模型
	var ums []model.UserModel
	model.DB.Preload("Model").Where("user_id = ?", id).Find(&ums)
	models := make([]modelBrief, len(ums))
	for i, um := range ums {
		if um.Model != nil {
			models[i] = modelBrief{ID: um.ModelID, Name: um.Model.Name, Slug: um.Model.Slug}
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"permissions": userPermissionResponse{
			Providers: providers,
			Models:    models,
		},
	})
}

// UpdateUserPermissions 更新用户的访问权限（admin 专用）。
func (h *UserHandler) UpdateUserPermissions(c *gin.Context) {
	id, err := middleware.GetID(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	var user model.User
	if err := model.DB.First(&user, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
		return
	}

	var req updateUserPermissionsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 事务更新权限
	tx := model.DB.Begin()

	// 清除旧权限
	tx.Where("user_id = ?", id).Delete(&model.UserProvider{})
	tx.Where("user_id = ?", id).Delete(&model.UserModel{})

	// 设置新厂商权限
	for _, pid := range req.ProviderIDs {
		if err := tx.Create(&model.UserProvider{UserID: uint(id), ProviderID: pid}).Error; err != nil {
			tx.Rollback()
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	}

	// 设置新模型权限
	for _, mid := range req.ModelIDs {
		if err := tx.Create(&model.UserModel{UserID: uint(id), ModelID: mid}).Error; err != nil {
			tx.Rollback()
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	}

	tx.Commit()

	c.JSON(http.StatusOK, gin.H{"message": "permissions updated"})
}

// ── 辅助函数 ──

// GetUserProviderIDs 获取用户被授权的厂商ID列表（供其他 handler 使用）。
func GetUserProviderIDs(userID uint) ([]uint, error) {
	var ups []model.UserProvider
	if err := model.DB.Where("user_id = ?", userID).Find(&ups).Error; err != nil {
		return nil, err
	}
	ids := make([]uint, len(ups))
	for i, up := range ups {
		ids[i] = up.ProviderID
	}
	return ids, nil
}

// GetUserModelIDs 获取用户被授权的映射模型ID列表（供其他 handler 使用）。
func GetUserModelIDs(userID uint) ([]uint, error) {
	var ums []model.UserModel
	if err := model.DB.Where("user_id = ?", userID).Find(&ums).Error; err != nil {
		return nil, err
	}
	ids := make([]uint, len(ums))
	for i, um := range ums {
		ids[i] = um.ModelID
	}
	return ids, nil
}

// getUserIDFromSession 从 session 中获取 user_id。
func getUserIDFromSession(c *gin.Context) (uint, bool) {
	userIDVal, exists := c.Get("user_id")
	if !exists {
		return 0, false
	}
	uid, ok := userIDVal.(uint)
	return uid, ok
}

// isAdmin 检查当前用户是否为 admin。
func IsAdmin(c *gin.Context) bool {
	roleVal, exists := c.Get("role")
	if !exists {
		return false
	}
	role, ok := roleVal.(string)
	return ok && role == "admin"
}

// GetCurrentUserID 从 context 中获取当前登录用户的 ID。
func GetCurrentUserID(c *gin.Context) uint {
	uid, _ := getUserIDFromSession(c)
	return uid
}

// parseIntParam 解析路径参数为 uint。
func parseIntParam(c *gin.Context, key string) (uint, error) {
	val, err := strconv.ParseUint(c.Param(key), 10, 32)
	if err != nil {
		return 0, err
	}
	return uint(val), nil
}
